package fetcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

const (
	// browserStopTimeout bounds the wait for one Chrome to exit after it was killed.
	browserStopTimeout = 3 * time.Second
	// poolCloseTimeout bounds the whole shutdown, so a stuck Chrome or a slow
	// disk cannot keep the server from exiting.
	poolCloseTimeout = 5 * time.Second
)

var errPoolClosed = errors.New("Chrome browser pool is shut down")

// browser is one headless Chrome process with its own profile directory.
type browser struct {
	ctx    context.Context
	cancel func()
	dir    string

	// Guarded by BrowserPool.mu.
	idleTimer *time.Timer
	idleGen   int

	closeOnce sync.Once
}

// alive reports whether Chrome is still running. chromedp cancels the browser
// context when the connection to Chrome is lost.
func (b *browser) alive() bool {
	return b.ctx.Err() == nil
}

// close kills Chrome and deletes its profile directory. Concurrent callers
// block until the first call has finished.
func (b *browser) close() {
	b.closeOnce.Do(func() {
		// Cancelling the context kills Chrome at once; the rest of cancel
		// waits for the process to be reaped.
		stopped := make(chan struct{})
		go func() {
			b.cancel()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(browserStopTimeout):
		}
		removeAllRetry(b.dir)
	})
}

// BrowserPool hands out Chrome instances. Chrome is started only when a fetch
// needs one, at most `size` run at a time, and instances left unused for
// idleTimeout are shut down again.
type BrowserPool struct {
	idleTimeout time.Duration
	slots       chan struct{} // one token per browser in use
	done        chan struct{} // closed by Close

	mu       sync.Mutex
	closed   bool
	session  *sessionDir           // exists only while there are browsers
	browsers map[*browser]struct{} // every browser not yet fully closed
	idle     []*browser            // launched and unused, most recently used last

	closeOnce sync.Once
}

// newBrowserPool creates an empty pool. idleTimeout <= 0 keeps idle browsers
// until Close.
func newBrowserPool(size int, idleTimeout time.Duration) *BrowserPool {
	return &BrowserPool{
		idleTimeout: idleTimeout,
		slots:       make(chan struct{}, size),
		done:        make(chan struct{}),
		browsers:    make(map[*browser]struct{}),
	}
}

// acquire returns a running browser, reusing an idle one when possible. It
// blocks while `size` browsers are in use. Every successful acquire must be
// paired with a release.
func (p *BrowserPool) acquire() (*browser, error) {
	select {
	case p.slots <- struct{}{}:
	case <-p.done:
		return nil, errPoolClosed
	}

	b, err := p.idleOrNewBrowser()
	if err != nil {
		<-p.slots
		return nil, err
	}
	return b, nil
}

func (p *BrowserPool) idleOrNewBrowser() (*browser, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errPoolClosed
	}

	for len(p.idle) > 0 {
		b := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		b.idleGen++
		if b.idleTimer != nil {
			b.idleTimer.Stop()
		}
		if b.alive() {
			p.mu.Unlock()
			return b, nil
		}
		// Chrome crashed or was killed from outside while idle.
		p.discardLocked(b)
	}

	// Register the browser before Chrome starts, so that a Close racing with
	// the launch still finds it and kills it.
	b, err := p.newBrowserLocked()
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}

	// The first Run on a browser context starts Chrome. It must be given the
	// browser context itself: Chrome is tied to the context passed here.
	if err := chromedp.Run(b.ctx); err != nil {
		p.mu.Lock()
		p.discardLocked(b)
		p.mu.Unlock()
		return nil, fmt.Errorf("failed to start Chrome: %w", err)
	}
	return b, nil
}

// newBrowserLocked prepares a browser with its own profile directory, without
// starting Chrome yet.
func (p *BrowserPool) newBrowserLocked() (*browser, error) {
	if p.session == nil {
		session, err := newSessionDir()
		if err != nil {
			return nil, fmt.Errorf("failed to create Chrome profile directory: %w", err)
		}
		p.session = session
	}

	dir, err := os.MkdirTemp(p.session.path, "profile-")
	if err != nil {
		return nil, fmt.Errorf("failed to create Chrome profile directory: %w", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-web-security", false),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent(types.DefaultUserAgent),
		// Our own profile directory instead of chromedp's temporary one: it
		// sits inside the session directory, so a later run can tell that it
		// was orphaned and remove it (see sweepStaleProfiles).
		chromedp.UserDataDir(dir),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)

	b := &browser{
		ctx: browserCtx,
		dir: dir,
		cancel: func() {
			browserCancel()
			allocCancel()
		},
	}
	p.browsers[b] = struct{}{}
	return b, nil
}

// release returns a browser to the pool after a fetch.
func (p *BrowserPool) release(b *browser) {
	defer func() { <-p.slots }()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	if !b.alive() {
		p.discardLocked(b)
		return
	}

	p.idle = append(p.idle, b)
	if p.idleTimeout > 0 {
		b.idleGen++
		gen := b.idleGen
		b.idleTimer = time.AfterFunc(p.idleTimeout, func() { p.reapIdle(b, gen) })
	}
}

// reapIdle shuts down a browser that stayed unused for idleTimeout. gen
// identifies the idle period the timer was started for; if the browser was
// used again since, the timer is stale and nothing happens.
func (p *BrowserPool) reapIdle(b *browser, gen int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || b.idleGen != gen {
		return
	}
	for i, candidate := range p.idle {
		if candidate == b {
			p.idle = append(p.idle[:i], p.idle[i+1:]...)
			p.discardLocked(b)
			return
		}
	}
}

// discardLocked closes a browser that is no longer in the idle list. The
// browser stays registered until it is fully closed, so that Close waits for
// it. When the last browser is gone the session directory goes too.
func (p *BrowserPool) discardLocked(b *browser) {
	go func() {
		b.close()

		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.browsers, b)
		if len(p.browsers) == 0 && p.session != nil && !p.closed {
			p.session.remove()
			p.session = nil
		}
	}()
}

// Close kills all Chrome instances and removes their profile directories. It
// returns when that is done or after poolCloseTimeout, whichever comes first.
// Concurrent and repeated calls are safe and all wait for the first one.
func (p *BrowserPool) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.done)
		browsers := make([]*browser, 0, len(p.browsers))
		for b := range p.browsers {
			browsers = append(browsers, b)
		}
		session := p.session
		p.mu.Unlock()

		finished := make(chan struct{})
		go func() {
			defer close(finished)
			var wg sync.WaitGroup
			for _, b := range browsers {
				wg.Add(1)
				go func(b *browser) {
					defer wg.Done()
					b.close()
				}(b)
			}
			wg.Wait()
			if session != nil {
				session.remove()
			}
		}()

		select {
		case <-finished:
		case <-time.After(poolCloseTimeout):
		}
	})
}
