package worker

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wiz/sendsmtp/internal/config"
	"github.com/wiz/sendsmtp/internal/mailer"
	"github.com/wiz/sendsmtp/internal/store"
)

type Progress struct {
	Sent    int64   `json:"sent"`
	Failed  int64   `json:"failed"`
	Pending int64   `json:"pending"`
	Rate    float64 `json:"rate"`
	State   string  `json:"state"`
}

type RatePoint struct {
	T    int64   `json:"t"`
	Rate float64 `json:"rate"`
	Sent int64   `json:"sent"`
}

type EventEmitter func(name string, data any)

type Runner struct {
	store   *store.Store
	cfg     config.Config
	emit    EventEmitter
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool

	sentThisWindow atomic.Int64
	windowStart    atomic.Int64

	histMu      sync.Mutex
	rateHistory []RatePoint
}

func New(st *store.Store, cfg config.Config, emit EventEmitter) *Runner {
	r := &Runner{store: st, cfg: cfg, emit: emit, rateHistory: make([]RatePoint, 0, 60)}
	r.windowStart.Store(time.Now().UnixNano())
	return r
}

func (r *Runner) UpdateConfig(cfg config.Config) {
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
}

func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return errors.New("campaign already running")
	}
	cfg := r.cfg
	if _, err := r.store.ReopenOrphans(); err != nil {
		return err
	}
	if err := r.store.StartCampaign(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.running = true
	r.windowStart.Store(time.Now().UnixNano())
	r.sentThisWindow.Store(0)
	r.histMu.Lock()
	r.rateHistory = r.rateHistory[:0]
	r.histMu.Unlock()

	go r.loop(ctx, cfg)
	return nil
}

func (r *Runner) Pause() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		_ = r.store.SetCampaignState("paused")
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.running = false
	return r.store.SetCampaignState("paused")
}

func (r *Runner) StopDone() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.running = false
}

func (r *Runner) loop(ctx context.Context, cfg config.Config) {
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	var wg sync.WaitGroup
	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	var rr atomic.Uint64
	emptyTicks := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, cfg, &rr)
		}()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			r.emitProgress("paused")
			return
		case <-ticker.C:
			r.emitProgress("running")
			idle, err := r.store.QueueIdle()
			if err != nil {
				continue
			}
			if idle {
				emptyTicks++
			} else {
				emptyTicks = 0
			}
			// Stay alive a bit so late imports keep flowing; finish after ~2s idle.
			if emptyTicks >= 4 {
				if r.cancel != nil {
					r.cancel()
				}
				wg.Wait()
				_ = r.store.SetCampaignState("done")
				r.emitProgress("done")
				if r.emit != nil {
					r.emit("campaign:done", map[string]any{"ok": true})
				}
				return
			}
		}
	}
}

func (r *Runner) worker(ctx context.Context, cfg config.Config, rr *atomic.Uint64) {
	dialTO := time.Duration(cfg.DialTimeoutSec) * time.Second
	sendTO := time.Duration(cfg.SendTimeoutSec) * time.Second
	backoff := time.Duration(cfg.RetryBackoffSec) * time.Second
	idleNoSMTP := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		camp, err := r.store.GetCampaign()
		if err != nil || camp.State != "running" {
			return
		}

		email, err := r.store.ClaimPendingEmail()
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Idle — do not exit; supervisor ends the campaign when queue stays empty.
				select {
				case <-ctx.Done():
					return
				case <-time.After(400 * time.Millisecond):
				}
				continue
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		nActive, _ := r.store.CountActiveSMTPs()
		if nActive == 0 {
			if n, _ := r.store.ReenableCooldownSMTPs(); n > 0 && r.emit != nil {
				r.emit("smtp:updated", map[string]any{"reenabled": n})
			}
			nActive, _ = r.store.CountActiveSMTPs()
			if nActive == 0 {
				_ = r.store.ReleaseToPending(email.ID)
				idleNoSMTP++
				wait := time.Duration(1+min(idleNoSMTP, 10)) * time.Second
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
				continue
			}
		}
		idleNoSMTP = 0

		offset := int(rr.Add(1) % uint64(nActive))
		acc, err := r.store.PickActiveSMTP(offset)
		if err != nil {
			_ = r.store.ReleaseToPending(email.ID)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		subject, err := r.store.RandomSubject()
		if err != nil || subject == "" {
			subject = "Hello"
		}
		link, _ := r.store.RandomLink()
		link = mailer.PersonalizeLink(link, email.Address)
		html := camp.HTML
		if html == "" {
			_ = r.store.MarkEmailFailed(email.ID, "empty HTML body", false)
			continue
		}

		from := mailer.ResolveFrom(acc.FromAddr, acc.User)
		if from == "" {
			from = acc.FromAddr // last resort for SMTP envelope; template still sanitizes {{from}}
		}
		subj := mailer.Prepare(subject, email.Address, link, subject, from)
		body := mailer.Prepare(html, email.Address, link, subj, from)

		msg := mailer.Message{
			FromName: cfg.FromName,
			From:     from,
			To:       email.Address,
			Subject:  subj,
			HTML:     body,
		}
		if camp.FromName != "" {
			msg.FromName = camp.FromName
		}

		sendErr := mailer.Send(acc, msg, dialTO, sendTO)
		if sendErr != nil {
			if mailer.ShouldPenalizeSMTP(sendErr) {
				disableAfter := cfg.SMTPDisableAfterFails
				if mailer.ClassifyError(sendErr) == mailer.ErrorSMTPFatal {
					disableAfter = 1
				}
				disabled, _ := r.store.RecordSMTPFailure(acc.ID, sendErr.Error(), disableAfter)
				if disabled && r.emit != nil {
					r.emit("smtp:updated", map[string]any{"id": acc.ID, "status": "disabled", "error": sendErr.Error()})
				}
			}
			retry := mailer.ShouldRetryEmail(sendErr, email.Attempts, cfg.RetryMax)
			_ = r.store.MarkEmailFailed(email.ID, sendErr.Error(), retry)
			if retry {
				time.Sleep(backoff)
			}
			continue
		}

		_ = r.store.RecordSMTPSuccess(acc.ID)
		_ = r.store.MarkEmailSent(email.ID, acc.ID, subj, link)
		r.sentThisWindow.Add(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (r *Runner) emitProgress(state string) {
	if r.emit == nil {
		return
	}
	st, err := r.store.GetStatus()
	if err != nil {
		return
	}
	now := time.Now().UnixNano()
	start := r.windowStart.Load()
	elapsed := float64(now-start) / 1e9
	rate := 0.0
	if elapsed > 0.2 {
		rate = float64(r.sentThisWindow.Load()) / elapsed
	}
	r.pushRate(now/1e6, rate, st.Sent)
	if elapsed > 5 {
		r.windowStart.Store(now)
		r.sentThisWindow.Store(0)
	}
	r.emit("campaign:progress", Progress{
		Sent:    st.Sent,
		Failed:  st.Failed,
		Pending: st.Pending,
		Rate:    rate,
		State:   state,
	})
}

func (r *Runner) pushRate(tMs int64, rate float64, sent int64) {
	r.histMu.Lock()
	defer r.histMu.Unlock()
	r.rateHistory = append(r.rateHistory, RatePoint{T: tMs, Rate: rate, Sent: sent})
	if len(r.rateHistory) > 120 {
		r.rateHistory = append([]RatePoint(nil), r.rateHistory[len(r.rateHistory)-120:]...)
	}
}

func (r *Runner) RateHistory() []RatePoint {
	r.histMu.Lock()
	defer r.histMu.Unlock()
	out := make([]RatePoint, len(r.rateHistory))
	copy(out, r.rateHistory)
	return out
}

func (r *Runner) CurrentProgress() Progress {
	st, _ := r.store.GetStatus()
	now := time.Now().UnixNano()
	start := r.windowStart.Load()
	elapsed := float64(now-start) / 1e9
	rate := 0.0
	if elapsed > 0.2 {
		rate = float64(r.sentThisWindow.Load()) / elapsed
	}
	state := "idle"
	if r.IsRunning() {
		state = "running"
	}
	return Progress{
		Sent:    st.Sent,
		Failed:  st.Failed,
		Pending: st.Pending,
		Rate:    rate,
		State:   state,
	}
}
