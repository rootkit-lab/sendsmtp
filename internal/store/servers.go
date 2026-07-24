package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Server is a remote VPS used as SOCKS5 egress for SMTP.
type Server struct {
	ID            int64  `json:"id"`
	Host          string `json:"host"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	SSHPassword   string `json:"ssh_password,omitempty"`
	PreferPort    int    `json:"prefer_port"`
	ProxyPort     int    `json:"proxy_port"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPassword string `json:"proxy_password,omitempty"`
	Status        string `json:"status"` // pending | active | disabled | error
	FailCount     int    `json:"fail_count"`
	LastError     string `json:"last_error"`
	SentCount     int64  `json:"sent_count"`
}

func scanServer(row interface{ Scan(dest ...any) error }, a *Server) error {
	return row.Scan(
		&a.ID, &a.Host, &a.SSHPort, &a.SSHUser, &a.SSHPassword,
		&a.PreferPort, &a.ProxyPort, &a.ProxyUser, &a.ProxyPassword,
		&a.Status, &a.FailCount, &a.LastError, &a.SentCount,
	)
}

const serverCols = `id, host, ssh_port, ssh_user, ssh_password, prefer_port, proxy_port, proxy_user, proxy_password, status, fail_count, last_error, sent_count`

func (s *Store) UpsertServers(list []Server) (inserted, updated int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, srv := range list {
		host := strings.TrimSpace(srv.Host)
		if host == "" {
			continue
		}
		if srv.SSHPort <= 0 {
			srv.SSHPort = 22
		}
		if srv.SSHUser == "" {
			srv.SSHUser = "root"
		}
		if srv.PreferPort <= 0 {
			srv.PreferPort = 18080
		}
		if srv.ProxyUser == "" {
			srv.ProxyUser = "sendsmtp"
		}

		status := srv.Status
		if status == "" {
			status = "pending"
		}
		proxyPort := srv.ProxyPort
		proxyPass := srv.ProxyPassword
		if proxyPass == "" {
			proxyPass = srv.SSHPassword
		}

		var id int64
		err := tx.QueryRow(`SELECT id FROM servers WHERE host=?`, host).Scan(&id)
		if err == sql.ErrNoRows {
			_, err = tx.Exec(`
INSERT INTO servers (host, ssh_port, ssh_user, ssh_password, prefer_port, proxy_port, proxy_user, proxy_password, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				host, srv.SSHPort, srv.SSHUser, srv.SSHPassword, srv.PreferPort,
				proxyPort, srv.ProxyUser, proxyPass, status)
			if err != nil {
				return inserted, updated, err
			}
			inserted++
			continue
		}
		if err != nil {
			return inserted, updated, err
		}
		if proxyPort > 0 {
			_, err = tx.Exec(`
UPDATE servers SET ssh_port=?, ssh_user=?, ssh_password=?, prefer_port=?,
  proxy_port=?, proxy_user=?, proxy_password=?, status=?, last_error='' WHERE id=?`,
				srv.SSHPort, srv.SSHUser, srv.SSHPassword, srv.PreferPort,
				proxyPort, srv.ProxyUser, proxyPass, status, id)
		} else {
			_, err = tx.Exec(`
UPDATE servers SET ssh_port=?, ssh_user=?, ssh_password=?, prefer_port=?, last_error='' WHERE id=?`,
				srv.SSHPort, srv.SSHUser, srv.SSHPassword, srv.PreferPort, id)
		}
		if err != nil {
			return inserted, updated, err
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return inserted, updated, nil
}

func (s *Store) ListServers() ([]Server, error) {
	rows, err := s.db.Query(`SELECT ` + serverCols + ` FROM servers ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Server
	for rows.Next() {
		var a Server
		if err := scanServer(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetServer(id int64) (Server, error) {
	var a Server
	err := scanServer(s.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE id=?`, id), &a)
	return a, err
}

func (s *Store) SetServerStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE servers SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) SetServerDeployed(id int64, proxyPort int, proxyUser, proxyPass string) error {
	_, err := s.db.Exec(`
UPDATE servers SET proxy_port=?, proxy_user=?, proxy_password=?, status='active', fail_count=0, last_error='' WHERE id=?`,
		proxyPort, proxyUser, proxyPass, id)
	return err
}

func (s *Store) SetServerError(id int64, errMsg string) error {
	_, err := s.db.Exec(`UPDATE servers SET status='error', last_error=? WHERE id=?`, errMsg, id)
	return err
}

func (s *Store) DeleteServer(id int64) error {
	_, err := s.db.Exec(`DELETE FROM servers WHERE id=?`, id)
	return err
}

func (s *Store) CountActiveServers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM servers WHERE status='active' AND proxy_port>0`).Scan(&n)
	return n, err
}

func (s *Store) PickActiveServer(offset int) (Server, error) {
	var a Server
	q := `SELECT ` + serverCols + ` FROM servers WHERE status='active' AND proxy_port>0 ORDER BY fail_count ASC, sent_count ASC, id ASC LIMIT 1 OFFSET ?`
	err := scanServer(s.db.QueryRow(q, offset), &a)
	if err == sql.ErrNoRows {
		err = scanServer(s.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE status='active' AND proxy_port>0 ORDER BY fail_count ASC, sent_count ASC, id ASC LIMIT 1`), &a)
	}
	return a, err
}

func (s *Store) RecordServerSuccess(id int64) error {
	_, err := s.db.Exec(`UPDATE servers SET fail_count=0, last_error='', sent_count=sent_count+1 WHERE id=?`, id)
	return err
}

func (s *Store) RecordServerFailure(id int64, errMsg string, disableAfter int) (disabled bool, err error) {
	_, err = s.db.Exec(`UPDATE servers SET fail_count=fail_count+1, last_error=? WHERE id=?`, errMsg, id)
	if err != nil {
		return false, err
	}
	var failCount int
	if err = s.db.QueryRow(`SELECT fail_count FROM servers WHERE id=?`, id).Scan(&failCount); err != nil {
		return false, err
	}
	if disableAfter > 0 && failCount >= disableAfter {
		if err = s.SetServerStatus(id, "disabled"); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *Store) SetServerPreferPort(id int64, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	_, err := s.db.Exec(`UPDATE servers SET prefer_port=? WHERE id=?`, port, id)
	return err
}
