package main

import (
	"fmt"

	"github.com/wiz/sendsmtp/internal/config"
	"github.com/wiz/sendsmtp/internal/engine"
	"github.com/wiz/sendsmtp/internal/inboxcheck"
	"github.com/wiz/sendsmtp/internal/store"
)

// AppService exposes engine methods to the React frontend.
type AppService struct {
	eng *engine.Engine
}

func NewAppService(eng *engine.Engine) *AppService {
	return &AppService{eng: eng}
}

func (a *AppService) ImportSmtpsText(raw string) (engine.ImportResult, error) {
	return a.eng.ImportSmtpsText(raw)
}

func (a *AppService) ImportEmailsText(raw string, validate bool) (engine.ImportResult, error) {
	return a.eng.ImportEmailsText(raw, validate)
}

func (a *AppService) ImportEmailsFile(path string, validate bool) (engine.ImportResult, error) {
	return a.eng.ImportEmailsFile(path, validate)
}

func (a *AppService) ImportSubjectsText(raw string) (engine.ImportResult, error) {
	return a.eng.ImportSubjectsText(raw)
}

func (a *AppService) ImportLinksText(raw string) (engine.ImportResult, error) {
	return a.eng.ImportLinksText(raw)
}

func (a *AppService) SetHtml(html string) error {
	return a.eng.SetHTML(html)
}

func (a *AppService) ImportAll() (map[string]engine.ImportResult, error) {
	return a.eng.ImportAll()
}

func (a *AppService) ListSmtps() ([]store.SMTP, error) {
	return a.eng.ListSmtps()
}

func (a *AppService) SetSmtpActive(id int64, active bool) error {
	return a.eng.SetSmtpActive(id, active)
}

func (a *AppService) TestSmtp(id int64, to string) error {
	return a.eng.TestSmtp(id, to)
}

func (a *AppService) AnalyzeSmtp(id int64) (inboxcheck.PlacementSummary, error) {
	return a.eng.AnalyzeSmtp(id)
}

func (a *AppService) AnalyzeAllSmtps() (engine.AnalyzeAllResult, error) {
	return a.eng.AnalyzeAllSmtps()
}

func (a *AppService) ExtractSmtpContacts(id int64) (engine.ExtractMailboxResult, error) {
	return a.eng.ExtractSmtpContacts(id)
}

func (a *AppService) ExtractAllSmtpContacts() (map[string]engine.ExtractMailboxResult, error) {
	return a.eng.ExtractAllSmtpContacts()
}

func (a *AppService) ListEmails(filter string, limit, offset int) ([]store.Email, error) {
	return a.eng.ListEmails(filter, limit, offset)
}

func (a *AppService) ListEmailsPage(filter string, query string, limit int, offset int) (store.EmailPage, error) {
	return a.eng.ListEmailsPage(filter, query, limit, offset)
}

func (a *AppService) ResetFailed() (int64, error) {
	return a.eng.ResetFailed()
}

func (a *AppService) DeleteAllEmails() (int64, error) {
	return a.eng.DeleteAllEmails()
}

func (a *AppService) ClearErrorLogs() (map[string]int64, error) {
	return a.eng.ClearErrorLogs()
}

func (a *AppService) ReenableSMTPs() (int64, error) {
	return a.eng.ReenableSMTPs()
}

func (a *AppService) GetConfig() config.Config {
	return a.eng.GetConfig()
}

func (a *AppService) SaveConfig(cfg config.Config) error {
	return a.eng.SaveConfig(cfg)
}

func (a *AppService) GetSubjects() ([]string, error) {
	return a.eng.GetSubjects()
}

func (a *AppService) GetLinks() ([]string, error) {
	return a.eng.GetLinks()
}

func (a *AppService) GetHtml() (string, error) {
	return a.eng.GetHTML()
}

func (a *AppService) StartCampaign() error {
	return a.eng.StartCampaign()
}

func (a *AppService) PauseCampaign() error {
	return a.eng.PauseCampaign()
}

func (a *AppService) ResumeCampaign() error {
	return a.eng.ResumeCampaign()
}

func (a *AppService) GetStatus() (store.StatusCounts, error) {
	return a.eng.GetStatus()
}

func (a *AppService) GetStats() (engine.Stats, error) {
	return a.eng.GetStats()
}

func (a *AppService) Ping() string {
	return fmt.Sprintf("sendsmtp ok")
}
