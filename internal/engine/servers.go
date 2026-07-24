package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wiz/sendsmtp/internal/proxyclient"
	"github.com/wiz/sendsmtp/internal/proxydeploy"
	"github.com/wiz/sendsmtp/internal/serverparse"
	"github.com/wiz/sendsmtp/internal/store"
)

// DeployServerResult is returned per-server after SOCKS deploy.
type DeployServerResult struct {
	ID      int64  `json:"id"`
	Host    string `json:"host"`
	OK      bool   `json:"ok"`
	Port    int    `json:"port"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func (e *Engine) ImportServersText(raw string) (ImportResult, error) {
	list, invalid := serverparse.ParseLines(raw)
	if len(list) == 0 {
		return ImportResult{Invalid: invalid}, fmt.Errorf("no valid servers (expected IP|password)")
	}
	ins, upd, err := e.Store.UpsertServers(list)
	if err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Inserted: ins, Updated: upd, Invalid: invalid}, nil
}

func (e *Engine) ListServers() ([]store.Server, error) {
	return e.Store.ListServers()
}

func (e *Engine) SetServerActive(id int64, active bool) error {
	status := "disabled"
	if active {
		srv, err := e.Store.GetServer(id)
		if err != nil {
			return err
		}
		if srv.ProxyPort > 0 {
			status = "active"
		} else {
			status = "pending"
		}
	}
	return e.Store.SetServerStatus(id, status)
}

func (e *Engine) DeleteServer(id int64) error {
	return e.Store.DeleteServer(id)
}

func (e *Engine) SetServerPreferPort(id int64, port int) error {
	return e.Store.SetServerPreferPort(id, port)
}

func (e *Engine) DeployServer(id int64) (DeployServerResult, error) {
	srv, err := e.Store.GetServer(id)
	if err != nil {
		return DeployServerResult{}, err
	}
	e.emitJob(JobProgress{
		Job: "server-deploy", Phase: "ssh", Percent: 5,
		Message: fmt.Sprintf("Deploy %s…", srv.Host), SMTPId: id,
	})
	res, err := proxydeploy.DeployWithProgress(srv, 25*time.Second, func(phase, message string) {
		pct := 10.0
		switch phase {
		case "ssh":
			pct = 15
		case "port":
			pct = 30
		case "upload":
			pct = 55
		case "start":
			pct = 85
		}
		e.emitJob(JobProgress{
			Job: "server-deploy", Phase: phase, Percent: pct,
			Message: message, SMTPId: id,
		})
	})
	out := DeployServerResult{ID: id, Host: srv.Host}
	if err != nil {
		_ = e.Store.SetServerError(id, err.Error())
		out.Error = err.Error()
		e.emitJob(JobProgress{Job: "server-deploy", Phase: "done", Percent: 100, Done: true, Message: err.Error(), SMTPId: id})
		return out, err
	}
	if err := e.Store.SetServerDeployed(id, res.ProxyPort, res.ProxyUser, res.ProxyPassword); err != nil {
		return out, err
	}
	out.OK = true
	out.Port = res.ProxyPort
	out.Message = res.Message
	e.emitJob(JobProgress{Job: "server-deploy", Phase: "done", Percent: 100, Done: true, Message: res.Message, SMTPId: id})
	return out, nil
}

func (e *Engine) DeployAllServers() ([]DeployServerResult, error) {
	list, err := e.Store.ListServers()
	if err != nil {
		return nil, err
	}

	type job struct {
		srv store.Server
		idx int
	}
	var jobs []job
	var skipped []DeployServerResult
	for i, srv := range list {
		if srv.Status == "disabled" {
			continue
		}
		if srv.ProxyPort > 0 && srv.SSHPassword == "" {
			skipped = append(skipped, DeployServerResult{
				ID: srv.ID, Host: srv.Host, OK: true, Port: srv.ProxyPort,
				Message: "already configured",
			})
			continue
		}
		// Already deployed: skip unless user wants redeploy — keep active, don't re-upload.
		if srv.ProxyPort > 0 && srv.Status == "active" {
			skipped = append(skipped, DeployServerResult{
				ID: srv.ID, Host: srv.Host, OK: true, Port: srv.ProxyPort,
				Message: "already active",
			})
			continue
		}
		jobs = append(jobs, job{srv: srv, idx: i})
	}

	total := len(jobs)
	if total == 0 {
		e.emitJob(JobProgress{Job: "server-deploy", Phase: "done", Percent: 100, Done: true, Message: "nothing to deploy"})
		return skipped, nil
	}

	const workers = 6
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var done atomic.Int64
	outMu := sync.Mutex{}
	out := make([]DeployServerResult, 0, total+len(skipped))
	out = append(out, skipped...)

	e.emitJob(JobProgress{
		Job: "server-deploy", Phase: "batch", Percent: 1,
		Current: 0, Total: total,
		Message: fmt.Sprintf("Deploy paralelo ×%d (%d hosts)…", workers, total),
	})

	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()

			e.emitJob(JobProgress{
				Job: "server-deploy", Phase: "batch",
				Percent: float64(done.Load()) / float64(total) * 100,
				Current: int(done.Load()) + 1, Total: total,
				Message: fmt.Sprintf("Deploy %s…", j.srv.Host),
				SMTPId:  j.srv.ID,
			})

			res, err := e.deployOneQuiet(j.srv.ID)
			if err != nil {
				res.Error = err.Error()
				res.OK = false
			}
			n := done.Add(1)
			outMu.Lock()
			out = append(out, res)
			outMu.Unlock()
			e.emitJob(JobProgress{
				Job: "server-deploy", Phase: "batch",
				Percent: float64(n) / float64(total) * 100,
				Current: int(n), Total: total,
				Message: fmt.Sprintf("%s — %s", j.srv.Host, pickMsg(res)),
				SMTPId:  j.srv.ID,
			})
		}(j)
	}
	wg.Wait()

	ok := 0
	for _, r := range out {
		if r.OK {
			ok++
		}
	}
	e.emitJob(JobProgress{
		Job: "server-deploy", Phase: "done", Percent: 100, Done: true,
		Message: fmt.Sprintf("deploy finished %d/%d OK", ok, len(out)),
	})
	return out, nil
}

// deployOneQuiet deploys without emitting Done on the single-server progress channel
// (batch owns progress). Still updates store.
func (e *Engine) deployOneQuiet(id int64) (DeployServerResult, error) {
	srv, err := e.Store.GetServer(id)
	if err != nil {
		return DeployServerResult{}, err
	}
	res, err := proxydeploy.Deploy(srv, 25*time.Second)
	out := DeployServerResult{ID: id, Host: srv.Host}
	if err != nil {
		_ = e.Store.SetServerError(id, err.Error())
		out.Error = err.Error()
		return out, err
	}
	if err := e.Store.SetServerDeployed(id, res.ProxyPort, res.ProxyUser, res.ProxyPassword); err != nil {
		return out, err
	}
	out.OK = true
	out.Port = res.ProxyPort
	out.Message = res.Message
	return out, nil
}

func pickMsg(r DeployServerResult) string {
	if r.OK {
		return r.Message
	}
	if r.Error != "" {
		return r.Error
	}
	return "fail"
}

func (e *Engine) TestServer(id int64) error {
	srv, err := e.Store.GetServer(id)
	if err != nil {
		return err
	}
	if srv.ProxyPort <= 0 {
		return fmt.Errorf("deploy SOCKS first")
	}
	dialTO := time.Duration(e.Cfg.DialTimeoutSec) * time.Second
	if dialTO <= 0 {
		dialTO = 15 * time.Second
	}
	conn, err := proxyclient.Dial(srv, "tcp", "1.1.1.1:80", dialTO)
	if err != nil {
		_ = e.Store.SetServerError(id, err.Error())
		return err
	}
	_ = conn.Close()
	_ = e.Store.SetServerStatus(id, "active")
	return nil
}
