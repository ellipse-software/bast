package ui

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/updater"
)

func (m *App) checkForUpdateCmd() tea.Cmd {
	var check func(context.Context, *http.Client, string) (string, error)
	switch {
	case updater.IsStable(m.version):
		check = updater.Check
	case updater.IsNightly(m.version):
		check = updater.CheckNightly
	default:
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		latest, err := check(ctx, &http.Client{Timeout: 4 * time.Second}, m.version)
		if err != nil || latest == "" {
			return nil
		}
		executable, _ := os.Executable()
		return updateAvailableMsg{version: latest, suggestion: updater.Suggestion(executable)}
	}
}

func (m *App) loadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		hosts, err := m.config.Discover()
		if err != nil {
			return loadedMsg{err: err}
		}

		identities := make([][]string, len(hosts))
		jobs := make(chan int)
		var workers sync.WaitGroup
		for range min(8, len(hosts)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					resolved, resolveErr := m.openSSH.Resolve(ctx, hosts[i].Alias)
					if resolveErr != nil {
						continue
					}
					hosts[i].Resolved = resolved
					known, _ := m.openSSH.Fingerprints(ctx, resolved.HostName, resolved.Port)
					hosts[i].KnownHost = known != ""
					identities[i] = resolved.IdentityFiles
				}
			}()
		}
		for i := range hosts {
			jobs <- i
		}
		close(jobs)
		workers.Wait()

		referenced := map[string][]string{}
		for i := range hosts {
			for _, identity := range identities[i] {
				referenced[identity] = append(referenced[identity], hosts[i].Alias)
			}
		}
		keyList, err := m.keyring.Discover(ctx, referenced)
		return loadedMsg{hosts: hosts, keys: keyList, err: err}
	}
}
