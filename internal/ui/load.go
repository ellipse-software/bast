package ui

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/sshconfig"
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
		hosts, err := m.config.Discover()
		if err != nil {
			return discoveredMsg{err: err}
		}
		return discoveredMsg{hosts: hosts}
	}
}

func (m *App) enrichCmd(discovered []sshconfig.Host) tea.Cmd {
	hosts := append([]sshconfig.Host(nil), discovered...)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		identities := make([][]string, len(hosts))
		jobs := make(chan int)
		var workers sync.WaitGroup
		for range min(8, len(hosts)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					if hosts[i].Resolved.HostName != "" || hosts[i].Resolved.User != "" {
						identities[i] = hosts[i].Resolved.IdentityFiles
						continue
					}
					resolved, resolveErr := m.openSSH.Resolve(ctx, hosts[i].Alias)
					if resolveErr != nil {
						continue
					}
					hosts[i].Resolved = resolved
					identities[i] = resolved.IdentityFiles
				}
			}()
		}
		for i := range hosts {
			jobs <- i
		}
		close(jobs)
		workers.Wait()

		type endpoint struct{ host, port string }
		indices := make(map[endpoint][]int, len(hosts))
		for i, host := range hosts {
			if host.Resolved.HostName != "" {
				key := endpoint{host: host.Resolved.HostName, port: host.Resolved.Port}
				indices[key] = append(indices[key], i)
			}
		}
		endpoints := make([]endpoint, 0, len(indices))
		for key := range indices {
			endpoints = append(endpoints, key)
		}
		knownHosts := make([]bool, len(endpoints))
		alreadyKnown := make([]bool, len(endpoints))
		for i, key := range endpoints {
			for _, hostIndex := range indices[key] {
				if hosts[hostIndex].KnownHost {
					alreadyKnown[i] = true
					knownHosts[i] = true
					break
				}
			}
		}
		knownJobs := make(chan int)
		for range min(8, len(endpoints)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range knownJobs {
					if alreadyKnown[i] {
						continue
					}
					known, _ := m.openSSH.Fingerprints(ctx, endpoints[i].host, endpoints[i].port)
					knownHosts[i] = known != ""
				}
			}()
		}
		for i := range endpoints {
			knownJobs <- i
		}
		close(knownJobs)
		workers.Wait()
		for i, key := range endpoints {
			for _, hostIndex := range indices[key] {
				hosts[hostIndex].KnownHost = knownHosts[i]
			}
		}

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
