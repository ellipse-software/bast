package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

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
		return filesListMsg{pane: index, cwd: cwd, entries: entries, err: err}
	}
}

func (m *App) refreshAllFilesPanes() tea.Cmd {
	return tea.Batch(m.refreshFilesPane(0), m.refreshFilesPane(1))
}

func (m *App) setFilesPaneLocal(index int) tea.Cmd {
	pane := &m.files.panes[index]
	pane.closeSession()
	pane.kind = filesPaneLocal
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	if pane.cwd == "" {
		pane.cwd = home
	} else if cleaned, cleanErr := files.CleanLocal(pane.cwd); cleanErr == nil {
		pane.cwd = cleaned
	} else {
		pane.cwd = home
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

func (m *App) connectFilesHost(index int, host sshconfig.Host) tea.Cmd {
	m.initFilesState()
	pane := &m.files.panes[index]
	pane.closeSession()
	pane.kind = filesPaneRemote
	pane.connecting = true
	pane.alias = host.Alias
	pane.err = ""
	openSSH := m.openSSH
	alias := host.Alias
	prepare := m.filesPrepareFn(host)
	return func() tea.Msg {
		if prepare != nil {
			if err := prepare(func(string) {}); err != nil {
				return filesConnectMsg{pane: index, alias: alias, err: err}
			}
		}
		session, err := files.OpenSession(openSSH, alias)
		if err != nil {
			return filesConnectMsg{pane: index, alias: alias, err: err}
		}
		cwd, err := session.Home()
		if err != nil {
			_ = session.Close()
			return filesConnectMsg{pane: index, alias: alias, err: err}
		}
		return filesConnectMsg{pane: index, session: session, cwd: cwd, alias: alias}
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
	}
	if ensure == nil {
		return func(func(string)) error {
			return m.metadata.RecordUse(host.Alias)
		}
	}
	return func(status func(string)) error {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	if msg.cwd != pane.cwd {
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
	pane.connecting = false
	if msg.err != nil {
		pane.alias = ""
		if msg.session != nil {
			_ = msg.session.Close()
		}
		notice := msg.err.Error()
		if formatted := openssh.FormatError(msg.err); formatted != "" && !strings.Contains(notice, formatted) {
			notice = formatted
		}
		return m.setNotice(notice)
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
	m.files.transfer.active = false
	m.files.transfer.cancel = nil
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
	m.files.transfer = filesTransfer{cancel: cancel, active: true, move: move}
	srcEnd := m.filesEndpoint(src)
	dstEnd := m.filesEndpoint(dst)
	dest := dst.cwd
	return m, func() tea.Msg {
		err := files.TransferAny(ctx, srcEnd, dstEnd, sources, dest, move, func(files.Progress) error {
			return ctx.Err()
		})
		return filesTransferDoneMsg{err: err, move: move}
	}
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
	m.openForm("Rename — "+entry.Name, "files_rename", []field{
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
		title = "Delete — " + confirm
	}
	m.openForm(title, "files_delete", []field{
		{label: "Pane", value: fmt.Sprintf("%d", m.files.focus), hidden: true},
		{label: "Paths", value: strings.Join(paths, "\n"), hidden: true},
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
		} else {
			path, err := files.JoinRemote(pane.cwd, name)
			if err != nil {
				return m.formError(err.Error())
			}
			if err := files.MkdirRemote(pane.session, path); err != nil {
				return m.formError(err.Error())
			}
		}
		m.form = nil
		return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Created"))
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
		} else {
			newPath, err := files.JoinRemote(pane.cwd, name)
			if err != nil {
				return m.formError(err.Error())
			}
			if err := files.RenameRemote(pane.session, oldPath, newPath); err != nil {
				return m.formError(err.Error())
			}
		}
		m.form = nil
		pane.clearMarks()
		return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Renamed"))
	case "files_delete":
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formValidationError("Name does not match")
		}
		for _, path := range strings.Split(values["Paths"], "\n") {
			if path == "" {
				continue
			}
			var err error
			if pane.kind == filesPaneLocal {
				err = files.RemoveLocal(path)
			} else {
				err = files.RemoveRemote(pane.session, path)
			}
			if err != nil {
				return m.formError(err.Error())
			}
		}
		m.form = nil
		pane.clearMarks()
		return m, tea.Batch(m.refreshFilesPane(index), m.setNotice("Deleted"))
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
	prepare := m.filesPrepareFn(host)
	telemetry.Track("files_shell", m.version)
	m.status = "Shell on " + pane.alias + " in " + pane.cwd
	return m, tea.Exec(&connectionProcess{cmd: cmd, prepare: prepare, version: m.version}, func(err error) tea.Msg {
		return processDoneMsg{name: "SSH session", err: err, sshSession: true}
	})
}
