package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	boxcloud "bast/internal/cloud/box"
	upstashcloud "bast/internal/cloud/upstash"
	"bast/internal/files"
	"bast/internal/openssh"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

func (m *App) refreshFilesPane(index int) tea.Cmd {
	m.initFilesState()
	pane := &m.files.panes[index]
	if pane.pickingHost() {
		return nil
	}
	if pane.kind == filesPaneRemote && pane.session == nil {
		return nil
	}
	pane.loading = true
	pane.err = ""
	pane.listGen++
	gen := pane.listGen
	cwd := pane.cwd
	showHidden := pane.showHidden
	session := pane.session
	local := pane.kind == filesPaneLocal
	return func() tea.Msg {
		var entries []files.Entry
		var err error
		if local {
			entries, err = files.ListLocal(cwd, showHidden)
		} else {
			entries, err = files.ListRemote(session, cwd, showHidden)
		}
		return filesListMsg{pane: index, cwd: cwd, gen: gen, entries: entries, err: err}
	}
}

func (m *App) refreshAllFilesPanes() tea.Cmd {
	return tea.Batch(m.refreshFilesPane(0), m.refreshFilesPane(1))
}

func filesConnectErrorNotice(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "Connect cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Connect timed out"
	}
	text := err.Error()
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "ssh-add") && strings.Contains(lower, "host"):
		return "Connect failed. Unlock with ssh-add, or Connect once from Hosts."
	case strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "authentication"),
		strings.Contains(lower, "publickey"),
		strings.Contains(lower, "no mutual signature"):
		return "Auth failed. Unlock the key with ssh-add, then retry."
	case strings.Contains(lower, "not in the list of known hosts"),
		strings.Contains(lower, "known_hosts"),
		strings.Contains(lower, "host key verification"):
		return "Host key unknown. Connect once from Hosts to accept it, then retry."
	case strings.Contains(lower, "ssh-add"),
		strings.Contains(lower, "accept the host key"):
		return "Connect failed. Unlock with ssh-add, or Connect once from Hosts."
	}
	if formatted := openssh.FormatError(err); formatted != "" {
		if formatted == "connection failed, refused, or interrupted" {
			return "Connect failed. Unlock with ssh-add, or Connect once from Hosts."
		}
		return formatted
	}
	return text
}

func (m *App) setFilesPaneLocal(index int) tea.Cmd {
	pane := &m.files.panes[index]
	saved := pane.cwd
	wasLocal := pane.kind == filesPaneLocal
	pane.closeSession()
	pane.kind = filesPaneLocal
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	pane.cwd = home
	if wasLocal && saved != "" {
		if cleaned, cleanErr := files.CleanLocal(saved); cleanErr == nil {
			pane.cwd = cleaned
		}
	}
	pane.hostSearch = ""
	return tea.Batch(m.refreshFilesPane(index), m.setNotice("Local"))
}

func (m *App) setFilesPaneRemote(index int) tea.Cmd {
	pane := &m.files.panes[index]
	if pane.connected() {
		return m.setNotice(pane.alias)
	}
	pane.closeSession()
	pane.kind = filesPaneRemote
	pane.hostSearch = ""
	pane.hostCursor = 0
	return m.setNotice("Pick a host")
}

func (m *App) disconnectFilesPane(index int) tea.Cmd {
	pane := &m.files.panes[index]
	pane.closeSession()
	pane.kind = filesPaneRemote
	pane.hostCursor = 0
	pane.hostSearch = ""
	return m.setNotice("Disconnected")
}

// cloudPrepareTimeout is the access-prep budget for GCP/AWS/Azure.
const cloudPrepareTimeout = 90 * time.Second

// boxPrepareTimeout covers resume (up to 3m), sync, and EnsureBoxAccess.
const boxPrepareTimeout = 5 * time.Minute

func prepareTimeoutForHost(host sshconfig.Host) time.Duration {
	if host.Synced && (host.SyncSource == "box" || host.SyncSource == "upstash" || host.SyncSource == "hetzner") {
		return boxPrepareTimeout
	}
	return cloudPrepareTimeout
}

func (m *App) connectFilesHost(index int, host sshconfig.Host) tea.Cmd {
	m.initFilesState()
	pane := &m.files.panes[index]
	pane.closeSession()
	pane.kind = filesPaneRemote
	pane.connecting = true
	pane.alias = host.Alias
	pane.err = ""
	pane.connectGen++
	gen := pane.connectGen
	ctx, cancel := context.WithTimeout(context.Background(), prepareTimeoutForHost(host))
	pane.connectCancel = cancel
	openSSH := m.openSSH
	alias := host.Alias
	prepare := m.filesPrepareFn(host)
	return func() tea.Msg {
		defer cancel()
		if prepare != nil {
			if err := prepare(func(string) {}); err != nil {
				return filesConnectMsg{pane: index, gen: gen, alias: alias, err: err}
			}
		}
		if err := ctx.Err(); err != nil {
			return filesConnectMsg{pane: index, gen: gen, alias: alias, err: err}
		}
		var session *files.Session
		var err error
		if host.Synced && host.SyncSource == "upstash" {
			session, err = files.OpenSessionPrepared(ctx, openSSH, alias, func(cmd *exec.Cmd) error {
				upstashcloud.PrepareSSH(cmd, "")
				return nil
			})
		} else {
			session, err = files.OpenSession(ctx, openSSH, alias)
		}
		if err != nil {
			return filesConnectMsg{pane: index, gen: gen, alias: alias, err: err}
		}
		cwd, err := session.Home()
		if err != nil {
			_ = session.Close()
			return filesConnectMsg{pane: index, gen: gen, alias: alias, err: err}
		}
		return filesConnectMsg{pane: index, gen: gen, session: session, cwd: cwd, alias: alias}
	}
}

func (m *App) filesPrepareFn(host sshconfig.Host) func(func(string)) error {
	if !host.Synced || host.SyncID == "" {
		return func(func(string)) error {
			return m.metadata.RecordUse(host.Alias)
		}
	}
	var ensure func(context.Context, sshconfig.Host, func(string)) error
	switch host.SyncSource {
	case "gcp":
		ensure = m.syncer.EnsureGCPAccess
	case "aws":
		ensure = m.syncer.EnsureAWSAccess
	case "azure":
		ensure = m.syncer.EnsureAzureAccess
	case "box":
		ensure = func(ctx context.Context, host sshconfig.Host, status func(string)) error {
			if m.hostLooksStopped(host) {
				if status != nil {
					status("Resuming box…")
				}
				// Resume and sync are separate so a post-resume sync failure
				// does not skip EnsureBoxAccess (which refreshes the SSH host).
				if err := m.syncer.Box.Resume(ctx, host.SyncID, boxcloud.ResumeOpts{}); err != nil {
					return err
				}
				_, _ = m.syncer.SyncBox(ctx)
			}
			return m.syncer.EnsureBoxAccess(ctx, host, status)
		}
	case "upstash":
		ensure = func(ctx context.Context, host sshconfig.Host, status func(string)) error {
			if m.hostLooksStopped(host) {
				if status != nil {
					status("Resuming Upstash box…")
				}
				if err := m.syncer.Upstash.Resume(ctx, host.SyncID); err != nil {
					return err
				}
				_, _ = m.syncer.SyncUpstash(ctx)
			}
			return m.syncer.EnsureUpstashAccess(ctx, host, status)
		}
	case "hetzner":
		ensure = func(ctx context.Context, host sshconfig.Host, status func(string)) error {
			if m.hostLooksStopped(host) {
				if status != nil {
					status("Starting Hetzner server…")
				}
				if _, err := m.syncer.StartHetzner(ctx, host.SyncID); err != nil {
					return err
				}
			}
			return m.syncer.EnsureHetznerAccess(ctx, host, status)
		}
	}
	if ensure == nil {
		return func(func(string)) error {
			return m.metadata.RecordUse(host.Alias)
		}
	}
	return func(status func(string)) error {
		ctx, cancel := context.WithTimeout(context.Background(), prepareTimeoutForHost(host))
		defer cancel()
		if err := ensure(ctx, host, status); err != nil {
			return err
		}
		return m.metadata.RecordUse(host.Alias)
	}
}

func (m *App) handleFilesListMsg(msg filesListMsg) tea.Cmd {
	if msg.pane < 0 || msg.pane > 1 {
		return nil
	}
	pane := &m.files.panes[msg.pane]
	if msg.gen != pane.listGen || msg.cwd != pane.cwd {
		return nil
	}
	pane.loading = false
	if msg.err != nil {
		pane.err = msg.err.Error()
		pane.entries = nil
		return nil
	}
	pane.err = ""
	pane.entries = msg.entries
	pane.clamp()
	return nil
}

func (m *App) handleFilesConnectMsg(msg filesConnectMsg) tea.Cmd {
	if msg.pane < 0 || msg.pane > 1 {
		return nil
	}
	pane := &m.files.panes[msg.pane]
	if msg.gen != pane.connectGen {
		if msg.session != nil {
			_ = msg.session.Close()
		}
		return nil
	}
	pane.connecting = false
	pane.connectCancel = nil
	if msg.err != nil {
		pane.alias = ""
		if msg.session != nil {
			_ = msg.session.Close()
		}
		notice := filesConnectErrorNotice(msg.err)
		return m.setNotice(notice)
	}
	if pane.session != nil {
		_ = pane.session.Close()
	}
	pane.session = msg.session
	pane.alias = msg.alias
	pane.cwd = msg.cwd
	pane.cursor = 0
	pane.clearMarks()
	m.files.focus = msg.pane
	telemetry.Track("files_connect", m.version)
	return tea.Batch(m.refreshFilesPane(msg.pane), m.setNotice(msg.alias))
}

func (m *App) handleFilesTransferDone(msg filesTransferDoneMsg) tea.Cmd {
	if msg.gen != 0 && msg.gen != m.files.transfer.gen {
		return nil
	}
	m.files.transfer.active = false
	m.files.transfer.cancel = nil
	m.files.transfer.progressCh = nil
	m.files.transfer.preparing = false
	cmds := []tea.Cmd{m.refreshAllFilesPanes()}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			cmds = append(cmds, m.setNotice("Cancelled"))
		} else {
			cmds = append(cmds, m.setNotice(msg.err.Error()))
		}
		return tea.Batch(cmds...)
	}
	action := "Copied"
	if msg.move {
		action = "Moved"
	}
	m.files.panes[0].clearMarks()
	m.files.panes[1].clearMarks()
	cmds = append(cmds, m.setNotice(action))
	return tea.Batch(cmds...)
}

func (m *App) handleFilesTransferProgress(msg filesTransferProgressMsg) tea.Cmd {
	if !m.files.transfer.active || msg.gen != m.files.transfer.gen {
		return m.listenFilesTransferProgress()
	}
	m.files.transfer.preparing = false
	m.files.transfer.name = msg.progress.CurrentName
	m.files.transfer.done = msg.progress.Done
	m.files.transfer.total = msg.progress.Total
	m.files.transfer.bytes = msg.progress.Bytes
	return m.listenFilesTransferProgress()
}

func (m *App) listenFilesTransferProgress() tea.Cmd {
	ch := m.files.transfer.progressCh
	gen := m.files.transfer.gen
	if ch == nil || !m.files.transfer.active {
		return nil
	}
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return filesTransferProgressMsg{gen: gen, progress: p}
	}
}

func sendFilesTransferProgress(ch chan files.Progress, p files.Progress) {
	if ch == nil {
		return
	}
	select {
	case ch <- p:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- p:
		default:
		}
	}
}

func (m *App) startFilesTransfer(move bool) (tea.Model, tea.Cmd) {
	src := m.filesFocusedPane()
	dst := m.filesOtherPane()
	if src.pickingHost() {
		return m, m.setNotice("Connect a host first")
	}
	if dst.pickingHost() {
		return m, m.setNotice("Other pane needs a path")
	}
	if src.kind == filesPaneRemote && src.session == nil {
		return m, m.setNotice("Connect a host first")
	}
	if dst.kind == filesPaneRemote && dst.session == nil {
		return m, m.setNotice("Other pane needs a path")
	}
	sources := src.selectedPaths()
	if len(sources) == 0 {
		return m, m.setNotice("Nothing selected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	progressCh := make(chan files.Progress, 1)
	gen := m.files.transfer.gen + 1
	m.files.transfer = filesTransfer{
		cancel:     cancel,
		active:     true,
		move:       move,
		gen:        gen,
		preparing:  true,
		progressCh: progressCh,
	}
	srcEnd := m.filesEndpoint(src)
	dstEnd := m.filesEndpoint(dst)
	dest := dst.cwd
	return m, tea.Batch(m.listenFilesTransferProgress(), func() tea.Msg {
		err := files.TransferAny(ctx, srcEnd, dstEnd, sources, dest, move, func(p files.Progress) error {
			sendFilesTransferProgress(progressCh, p)
			return ctx.Err()
		})
		close(progressCh)
		return filesTransferDoneMsg{err: err, move: move, gen: gen}
	})
}

func (m *App) handleFilesOpDone(msg filesOpDoneMsg) tea.Cmd {
	if msg.pane < 0 || msg.pane > 1 {
		return nil
	}
	pane := &m.files.panes[msg.pane]
	if msg.err != nil {
		return m.setNotice(msg.err.Error())
	}
	pane.clearMarks()
	m.files.deletePaths = nil
	notice := msg.notice
	if notice == "" {
		notice = "Done"
	}
	return tea.Batch(m.refreshFilesPane(msg.pane), m.setNotice(notice))
}

func (m *App) openFilesMkdirForm() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	m.openForm("New directory", "files_mkdir", []field{
		{label: "Pane", value: fmt.Sprintf("%d", m.files.focus), hidden: true},
		{label: "Name", placeholder: "dirname"},
	})
	return m, nil
}

func (m *App) openFilesRenameForm() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	entry, ok := pane.selectedEntry()
	if !ok {
		return m, m.setNotice("Nothing selected")
	}
	m.openForm("Rename: "+entry.Name, "files_rename", []field{
		{label: "Pane", value: fmt.Sprintf("%d", m.files.focus), hidden: true},
		{label: "Path", value: entry.Path, hidden: true},
		{label: "Name", value: entry.Name, placeholder: entry.Name},
	})
	return m, nil
}

func (m *App) openFilesDeleteForm() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	paths := pane.selectedPaths()
	if len(paths) == 0 {
		return m, m.setNotice("Nothing selected")
	}
	confirm := "DELETE"
	placeholder := "DELETE"
	title := fmt.Sprintf("Delete %d items", len(paths))
	if len(paths) == 1 {
		confirm = files.BaseName(paths[0])
		placeholder = confirm
		title = "Delete: " + confirm
	}
	m.files.deletePaths = append([]string(nil), paths...)
	m.openForm(title, "files_delete", []field{
		{label: "Pane", value: fmt.Sprintf("%d", m.files.focus), hidden: true},
		{label: "Confirmation", value: confirm, hidden: true},
		{label: "Type the name to confirm", placeholder: placeholder},
	})
	return m, nil
}

func (m *App) submitFilesForm(values map[string]string) (tea.Model, tea.Cmd) {
	index := 0
	if values["Pane"] == "1" {
		index = 1
	}
	pane := &m.files.panes[index]
	switch m.form.action {
	case "files_mkdir":
		name := strings.TrimSpace(values["Name"])
		if name == "" {
			return m.formError("name is required")
		}
		if pane.kind == filesPaneLocal {
			path, err := files.JoinLocal(pane.cwd, name)
			if err != nil {
				return m.formError(err.Error())
			}
			if err := files.MkdirLocal(path); err != nil {
				return m.formError(err.Error())
			}
			m.form = nil
			return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Created"))
		}
		if pane.session == nil {
			return m.formError("not connected")
		}
		path, err := files.JoinRemote(pane.cwd, name)
		if err != nil {
			return m.formError(err.Error())
		}
		session := pane.session
		m.form = nil
		return m, func() tea.Msg {
			err := files.MkdirRemote(session, path)
			return filesOpDoneMsg{pane: index, action: "files_mkdir", err: err, notice: "Created"}
		}
	case "files_rename":
		name := strings.TrimSpace(values["Name"])
		if name == "" {
			return m.formError("name is required")
		}
		oldPath := values["Path"]
		if pane.kind == filesPaneLocal {
			newPath, err := files.JoinLocal(pane.cwd, name)
			if err != nil {
				return m.formError(err.Error())
			}
			if err := files.RenameLocal(oldPath, newPath); err != nil {
				return m.formError(err.Error())
			}
			m.form = nil
			pane.clearMarks()
			return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Renamed"))
		}
		if pane.session == nil {
			return m.formError("not connected")
		}
		newPath, err := files.JoinRemote(pane.cwd, name)
		if err != nil {
			return m.formError(err.Error())
		}
		session := pane.session
		m.form = nil
		return m, func() tea.Msg {
			err := files.RenameRemote(session, oldPath, newPath)
			return filesOpDoneMsg{pane: index, action: "files_rename", err: err, notice: "Renamed"}
		}
	case "files_delete":
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formValidationError("Name does not match")
		}
		paths := append([]string(nil), m.files.deletePaths...)
		if len(paths) == 0 {
			return m.formError("nothing to delete")
		}
		if pane.kind == filesPaneLocal {
			for _, path := range paths {
				if err := files.RemoveLocal(path); err != nil {
					return m.formError(err.Error())
				}
			}
			m.form = nil
			m.files.deletePaths = nil
			pane.clearMarks()
			return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Deleted"))
		}
		if pane.session == nil {
			return m.formError("not connected")
		}
		session := pane.session
		m.form = nil
		m.files.deletePaths = nil
		return m, func() tea.Msg {
			for _, path := range paths {
				if err := files.RemoveRemote(session, path); err != nil {
					return filesOpDoneMsg{pane: index, action: "files_delete", err: err}
				}
			}
			return filesOpDoneMsg{pane: index, action: "files_delete", notice: "Deleted"}
		}
	}
	return m, nil
}

func (m *App) filesOpenShell() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.kind == filesPaneLocal {
		cmd, err := files.ShellCommand(pane.cwd)
		if err != nil {
			m.setError(err)
			return m, nil
		}
		m.status = "Shell in " + pane.cwd
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return processDoneMsg{name: "Local shell", err: err}
		})
	}
	if pane.pickingHost() {
		hosts := m.filesHostList(pane)
		if pane.hostCursor < 0 || pane.hostCursor >= len(hosts) {
			return m, m.setNotice("Select a host first")
		}
		host := hosts[pane.hostCursor]
		return m.startSSH(host, m.filesPrepareFn(host))
	}
	host, ok := m.findHost(pane.alias)
	if !ok {
		host = sshconfig.Host{Alias: pane.alias}
	}
	cmd, err := m.openSSH.SSHCommandInDir(pane.alias, pane.cwd)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	if host.Synced && host.SyncSource == "upstash" {
		upstashcloud.PrepareSSH(cmd, m.syncer.BastExecutable)
	}
	prepare := m.filesPrepareFn(host)
	telemetry.Track("files_shell", m.version)
	m.status = "Shell on " + pane.alias + " in " + pane.cwd
	return m, tea.Exec(&connectionProcess{cmd: cmd, prepare: prepare, version: m.version}, func(err error) tea.Msg {
		return processDoneMsg{name: "SSH session", err: err, sshSession: true}
	})
}
