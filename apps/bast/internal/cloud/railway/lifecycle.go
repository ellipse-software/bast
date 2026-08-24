package railway

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CreateOpts struct {
	ProjectID     string
	NewProject    string
	EnvironmentID string
	Name          string
	Image         string
	StartCommand  string
}

func (c *Client) Create(ctx context.Context, opts CreateOpts) (Instance, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return Instance{}, fmt.Errorf("service name is required")
	}
	image := strings.TrimSpace(opts.Image)
	if image == "" {
		image = DefaultImage
	}
	start := strings.TrimSpace(opts.StartCommand)
	if start == "" {
		start = DefaultStart
	}
	projectID := strings.TrimSpace(opts.ProjectID)
	envID := strings.TrimSpace(opts.EnvironmentID)
	if newName := strings.TrimSpace(opts.NewProject); newName != "" {
		created, err := c.createProject(ctx, newName)
		if err != nil {
			return Instance{}, err
		}
		projectID = created.ID
		if envID == "" {
			envID = defaultEnvironmentID(created)
		}
	}
	if projectID == "" {
		return Instance{}, fmt.Errorf("project is required")
	}
	project, err := c.getProject(ctx, projectID)
	if err != nil {
		return Instance{}, err
	}
	if envID == "" {
		envID = defaultEnvironmentID(project)
	}
	if envID == "" {
		return Instance{}, fmt.Errorf("project %s has no environments", project.Name)
	}
	var created struct {
		ServiceCreate *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"serviceCreate"`
	}
	if err := c.graphql(ctx, `mutation serviceCreate($input: ServiceCreateInput!) {
  serviceCreate(input: $input) { id name }
}`, map[string]any{
		"input": map[string]any{
			"projectId": project.ID,
			"name":      name,
			"source":    map[string]any{"image": image},
		},
	}, &created); err != nil {
		return Instance{}, err
	}
	if created.ServiceCreate == nil || strings.TrimSpace(created.ServiceCreate.ID) == "" {
		return Instance{}, fmt.Errorf("railway service create: no id in response")
	}
	serviceID := created.ServiceCreate.ID
	if err := c.graphql(ctx, `mutation serviceInstanceUpdate($serviceId: String!, $environmentId: String!, $input: ServiceInstanceUpdateInput!) {
  serviceInstanceUpdate(serviceId: $serviceId, environmentId: $environmentId, input: $input)
}`, map[string]any{
		"serviceId":     serviceID,
		"environmentId": envID,
		"input":         map[string]any{"startCommand": start},
	}, nil); err != nil {
		return Instance{}, err
	}
	if err := c.graphql(ctx, `mutation serviceInstanceDeployV2($serviceId: String!, $environmentId: String!) {
  serviceInstanceDeployV2(serviceId: $serviceId, environmentId: $environmentId)
}`, map[string]any{
		"serviceId":     serviceID,
		"environmentId": envID,
	}, nil); err != nil {
		return Instance{}, err
	}
	syncID := FormatSyncID(project.ID, envID, serviceID)
	if err := c.WaitReady(ctx, syncID, 5*time.Minute); err != nil {
		inst, _ := c.GetInstance(ctx, syncID)
		return inst, err
	}
	return c.GetInstance(ctx, syncID)
}

func (c *Client) createProject(ctx context.Context, name string) (Project, error) {
	var data struct {
		ProjectCreate *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projectCreate"`
	}
	if err := c.graphql(ctx, `mutation projectCreate($input: ProjectCreateInput!) {
  projectCreate(input: $input) { id name }
}`, map[string]any{"input": map[string]any{"name": name}}, &data); err != nil {
		return Project{}, err
	}
	if data.ProjectCreate == nil || strings.TrimSpace(data.ProjectCreate.ID) == "" {
		return Project{}, fmt.Errorf("railway project create: no id in response")
	}
	return c.getProject(ctx, data.ProjectCreate.ID)
}

func (c *Client) getProject(ctx context.Context, id string) (Project, error) {
	var data struct {
		Project *projectNode `json:"project"`
	}
	if err := c.graphql(ctx, `query project($id: String!) {
  project(id: $id) {
    id
    name
    environments { edges { node { id name } } }
  }
}`, map[string]any{"id": id}, &data); err != nil {
		return Project{}, err
	}
	if data.Project == nil || strings.TrimSpace(data.Project.ID) == "" {
		return Project{}, fmt.Errorf("railway project %s was not found", id)
	}
	project := Project{ID: strings.TrimSpace(data.Project.ID), Name: strings.TrimSpace(data.Project.Name)}
	if project.Name == "" {
		project.Name = project.ID
	}
	for _, env := range nodes(data.Project.Environments) {
		envID := strings.TrimSpace(env.ID)
		if envID == "" {
			continue
		}
		name := strings.TrimSpace(env.Name)
		if name == "" {
			name = envID
		}
		project.Environments = append(project.Environments, Environment{ID: envID, Name: name})
	}
	return project, nil
}

func (c *Client) ResolveProject(ctx context.Context, idOrName string) (Project, error) {
	idOrName = strings.TrimSpace(idOrName)
	if idOrName == "" {
		return Project{}, fmt.Errorf("project is required")
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return Project{}, err
	}
	var matches []Project
	for _, project := range projects {
		if project.ID == idOrName || strings.EqualFold(project.Name, idOrName) {
			matches = append(matches, project)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Project{}, fmt.Errorf("project %q matches %d projects; pass a project id", idOrName, len(matches))
	}
	return Project{}, fmt.Errorf("railway project %q not found", idOrName)
}

func defaultEnvironmentID(project Project) string {
	if len(project.Environments) == 0 {
		return ""
	}
	for _, env := range project.Environments {
		if strings.EqualFold(env.Name, "production") {
			return env.ID
		}
	}
	return project.Environments[0].ID
}

func (c *Client) Stop(ctx context.Context, syncID string) error {
	inst, err := c.GetInstance(ctx, syncID)
	if err != nil {
		return err
	}
	if IsStoppedState(inst.State) {
		return fmt.Errorf("railway service %s is already stopped", inst.Name)
	}
	if inst.DeploymentID == "" {
		return fmt.Errorf("railway service %s has no deployment to stop", inst.Name)
	}
	if err := c.graphql(ctx, `mutation deploymentStop($id: String!) { deploymentStop(id: $id) }`, map[string]any{"id": inst.DeploymentID}, nil); err != nil {
		return err
	}
	return c.WaitStopped(ctx, inst.SyncID, 3*time.Minute)
}

func (c *Client) Resume(ctx context.Context, syncID string) error {
	inst, err := c.GetInstance(ctx, syncID)
	if err != nil {
		return err
	}
	if isReadyState(inst.State) {
		return nil
	}
	_, envID, serviceID, err := ParseSyncID(inst.SyncID)
	if err != nil {
		return err
	}
	redeployErr := c.graphql(ctx, `mutation serviceInstanceRedeploy($serviceId: String!, $environmentId: String!) {
  serviceInstanceRedeploy(serviceId: $serviceId, environmentId: $environmentId)
}`, map[string]any{"serviceId": serviceID, "environmentId": envID}, nil)
	if redeployErr != nil {
		if err := c.graphql(ctx, `mutation serviceInstanceDeployV2($serviceId: String!, $environmentId: String!) {
  serviceInstanceDeployV2(serviceId: $serviceId, environmentId: $environmentId)
}`, map[string]any{"serviceId": serviceID, "environmentId": envID}, nil); err != nil {
			return redeployErr
		}
	}
	return c.WaitReady(ctx, inst.SyncID, 5*time.Minute)
}

func (c *Client) Delete(ctx context.Context, syncID string) error {
	_, _, serviceID, err := ParseSyncID(syncID)
	if err != nil {
		return err
	}
	return c.graphql(ctx, `mutation serviceDelete($id: String!) { serviceDelete(id: $id) }`, map[string]any{"id": serviceID}, nil)
}

func (c *Client) WaitReady(ctx context.Context, syncID string, timeout time.Duration) error {
	return c.waitState(ctx, syncID, "ready", timeout, isReadyState, []string{"error"})
}

func (c *Client) WaitStopped(ctx context.Context, syncID string, timeout time.Duration) error {
	return c.waitState(ctx, syncID, "stopped", timeout, IsStoppedState, nil)
}

func (c *Client) waitState(ctx context.Context, syncID, timeoutLabel string, timeout time.Duration, ready func(string) bool, fail []string) error {
	deadline := time.Now().Add(timeout)
	var last string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for railway service %s to become %s (last state %s): %w", syncID, timeoutLabel, last, lastErr)
			}
			if last == "" {
				return fmt.Errorf("timed out waiting for railway service %s to become %s", syncID, timeoutLabel)
			}
			return fmt.Errorf("timed out waiting for railway service %s to become %s (last state %s)", syncID, timeoutLabel, last)
		}
		inst, err := c.GetInstance(ctx, syncID)
		if err == nil {
			lastErr = nil
			last = inst.State
			if ready(inst.State) {
				return nil
			}
			for _, bad := range fail {
				if last == bad {
					return fmt.Errorf("railway service %s entered %s state", inst.Name, last)
				}
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}
