package railway

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Instance struct {
	SyncID          string
	Name            string
	ProjectID       string
	ProjectName     string
	EnvironmentID   string
	EnvironmentName string
	ServiceID       string
	ServiceInstance string
	DeploymentID    string
	State           string
	HostName        string
	User            string
	Running         bool
	IdentityFile    string
	Tags            []string
}

type Project struct {
	ID           string
	Name         string
	Environments []Environment
}

type Environment struct {
	ID   string
	Name string
}

type Discovery struct {
	Instances []Instance
	Projects  []Project
	Warnings  []string
	Complete  bool
}

type projectNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Environments edgeList[envNode] `json:"environments"`
}

type envNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type serviceInstanceNode struct {
	ID               string          `json:"id"`
	ServiceID        string          `json:"serviceId"`
	ServiceName      string          `json:"serviceName"`
	LatestDeployment *deploymentNode `json:"latestDeployment"`
}

type deploymentNode struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	DeploymentStopped bool   `json:"deploymentStopped"`
}

const (
	railwayPageSize = 100
	railwayMaxPages = 50
)

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var raw []projectNode
	after := ""
	for page := 0; page < railwayMaxPages; page++ {
		var data struct {
			Projects edgeList[projectNode] `json:"projects"`
		}
		vars := map[string]any{"first": railwayPageSize}
		if after != "" {
			vars["after"] = after
		}
		if err := c.graphql(ctx, `query Projects($first: Int!, $after: String) {
  projects(first: $first, after: $after) {
    edges {
      node {
        id
        name
        environments(first: 100) {
          edges { node { id name } }
          pageInfo { hasNextPage endCursor }
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}`, vars, &data); err != nil {
			return nil, err
		}
		raw = append(raw, nodes(data.Projects)...)
		next, ok := nextCursor(data.Projects.PageInfo, after)
		if !ok {
			after = ""
			break
		}
		after = next
		if page == railwayMaxPages-1 {
			return nil, fmt.Errorf("railway project list truncated after %d pages", railwayMaxPages)
		}
	}
	out := make([]Project, 0, len(raw))
	for _, node := range raw {
		project, err := c.projectFromNode(ctx, node)
		if err != nil {
			return nil, err
		}
		if project.ID == "" {
			continue
		}
		out = append(out, project)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (c *Client) projectFromNode(ctx context.Context, node projectNode) (Project, error) {
	project := Project{ID: strings.TrimSpace(node.ID), Name: strings.TrimSpace(node.Name)}
	if project.ID == "" {
		return Project{}, nil
	}
	if project.Name == "" {
		project.Name = project.ID
	}
	envs := node.Environments
	after := ""
	for page := 0; page < railwayMaxPages; page++ {
		project.Environments = appendEnvironments(project.Environments, envs)
		next, ok := nextCursor(envs.PageInfo, after)
		if !ok {
			return project, nil
		}
		var data struct {
			Project *struct {
				Environments edgeList[envNode] `json:"environments"`
			} `json:"project"`
		}
		if err := c.graphql(ctx, `query ProjectEnvironments($id: String!, $first: Int!, $after: String) {
  project(id: $id) {
    environments(first: $first, after: $after) {
      edges { node { id name } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, map[string]any{"id": project.ID, "first": railwayPageSize, "after": next}, &data); err != nil {
			return Project{}, err
		}
		if data.Project == nil {
			return Project{}, fmt.Errorf("railway project %s environments missing", project.Name)
		}
		after = next
		envs = data.Project.Environments
		if page == railwayMaxPages-1 {
			return Project{}, fmt.Errorf("railway environments for %s truncated after %d pages", project.Name, railwayMaxPages)
		}
	}
	return project, nil
}

func appendEnvironments(dst []Environment, list edgeList[envNode]) []Environment {
	for _, env := range nodes(list) {
		id := strings.TrimSpace(env.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(env.Name)
		if name == "" {
			name = id
		}
		dst = append(dst, Environment{ID: id, Name: name})
	}
	return dst
}

func nextCursor(info *struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}, previous string) (string, bool) {
	if info == nil || !info.HasNextPage {
		return "", false
	}
	cursor := strings.TrimSpace(info.EndCursor)
	if cursor == "" || cursor == previous {
		return "", false
	}
	return cursor, true
}

func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	account, err := c.Account(ctx)
	if err != nil {
		return Discovery{}, err
	}
	if !account.Authenticated {
		msg := account.Error
		if msg == "" {
			msg = "not authenticated; connect on the Sync tab or set " + APIKeyEnv
		}
		return Discovery{}, fmt.Errorf("%s", msg)
	}
	projects, err := c.ListProjects(ctx)
	if err != nil {
		return Discovery{}, err
	}
	identity := c.identityPath()
	instances := make([]Instance, 0)
	var warnings []string
	complete := true
	for _, project := range projects {
		for _, env := range project.Environments {
			envInstances, truncated, envErr := c.listEnvironmentServices(ctx, project, env)
			if envErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s/%s: %s", project.Name, env.Name, envErr.Error()))
				complete = false
				continue
			}
			if truncated {
				warnings = append(warnings, fmt.Sprintf("%s/%s: service list truncated", project.Name, env.Name))
				complete = false
			}
			for i := range envInstances {
				if identity != "" {
					envInstances[i].IdentityFile = identity
				}
			}
			instances = append(instances, envInstances...)
		}
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Running != instances[j].Running {
			return instances[i].Running
		}
		if instances[i].ProjectName != instances[j].ProjectName {
			return instances[i].ProjectName < instances[j].ProjectName
		}
		if instances[i].EnvironmentName != instances[j].EnvironmentName {
			return instances[i].EnvironmentName < instances[j].EnvironmentName
		}
		return instances[i].Name < instances[j].Name
	})
	return Discovery{Instances: instances, Projects: projects, Warnings: warnings, Complete: complete}, nil
}

func (c *Client) listEnvironmentServices(ctx context.Context, project Project, env Environment) ([]Instance, bool, error) {
	var out []Instance
	after := ""
	for page := 0; page < railwayMaxPages; page++ {
		var data struct {
			Environment *struct {
				ServiceInstances edgeList[serviceInstanceNode] `json:"serviceInstances"`
			} `json:"environment"`
		}
		vars := map[string]any{
			"environmentId": env.ID,
			"projectId":     project.ID,
			"first":         railwayPageSize,
		}
		if after != "" {
			vars["after"] = after
		}
		if err := c.graphql(ctx, `query EnvironmentServices($environmentId: String!, $projectId: String!, $first: Int, $after: String) {
  environment(id: $environmentId, projectId: $projectId) {
    serviceInstances(first: $first, after: $after) {
      edges {
        node {
          id
          serviceId
          serviceName
          latestDeployment { id status deploymentStopped }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, vars, &data); err != nil {
			return nil, false, err
		}
		if data.Environment == nil {
			return out, false, nil
		}
		for _, node := range nodes(data.Environment.ServiceInstances) {
			inst, ok := instanceFromService(project, env, node)
			if ok {
				out = append(out, inst)
			}
		}
		next, ok := nextCursor(data.Environment.ServiceInstances.PageInfo, after)
		if !ok {
			return out, false, nil
		}
		after = next
	}
	return out, true, nil
}

func instanceFromService(project Project, env Environment, node serviceInstanceNode) (Instance, bool) {
	serviceID := strings.TrimSpace(node.ServiceID)
	if serviceID == "" {
		return Instance{}, false
	}
	name := strings.TrimSpace(node.ServiceName)
	if name == "" {
		name = serviceID
	}
	user := strings.TrimSpace(node.ID)
	state := "stopped"
	deployID := ""
	if node.LatestDeployment != nil {
		deployID = strings.TrimSpace(node.LatestDeployment.ID)
		state = normalizeState(node.LatestDeployment.Status, node.LatestDeployment.DeploymentStopped)
	}
	running := isVisibleRunning(state)
	tags := []string{"state:" + state, "project:" + project.ID, "environment:" + env.Name}
	return Instance{
		SyncID:          FormatSyncID(project.ID, env.ID, serviceID),
		Name:            name,
		ProjectID:       project.ID,
		ProjectName:     project.Name,
		EnvironmentID:   env.ID,
		EnvironmentName: env.Name,
		ServiceID:       serviceID,
		ServiceInstance: user,
		DeploymentID:    deployID,
		State:           state,
		HostName:        SSHHost,
		User:            user,
		Running:         running,
		Tags:            tags,
	}, true
}

func (c *Client) GetInstance(ctx context.Context, syncID string) (Instance, error) {
	projectID, envID, serviceID, err := ParseSyncID(syncID)
	if err != nil {
		return Instance{}, err
	}
	var data struct {
		ServiceInstance *serviceInstanceNode `json:"serviceInstance"`
		Project         *struct {
			ID           string            `json:"id"`
			Name         string            `json:"name"`
			Environments edgeList[envNode] `json:"environments"`
		} `json:"project"`
	}
	if err := c.graphql(ctx, `query ServiceAccess($serviceId: String!, $environmentId: String!, $projectId: String!) {
  serviceInstance(serviceId: $serviceId, environmentId: $environmentId) {
    id
    serviceId
    serviceName
    latestDeployment { id status deploymentStopped }
  }
  project(id: $projectId) {
    id
    name
    environments { edges { node { id name } } }
  }
}`, map[string]any{
		"serviceId":     serviceID,
		"environmentId": envID,
		"projectId":     projectID,
	}, &data); err != nil {
		return Instance{}, err
	}
	if data.ServiceInstance == nil {
		return Instance{}, fmt.Errorf("railway service %s was not found", serviceID)
	}
	project := Project{ID: projectID, Name: projectID}
	env := Environment{ID: envID, Name: envID}
	if data.Project != nil {
		project.ID = strings.TrimSpace(data.Project.ID)
		project.Name = strings.TrimSpace(data.Project.Name)
		if project.Name == "" {
			project.Name = project.ID
		}
		for _, node := range nodes(data.Project.Environments) {
			if strings.TrimSpace(node.ID) == envID {
				env.Name = strings.TrimSpace(node.Name)
				if env.Name == "" {
					env.Name = envID
				}
				break
			}
		}
	}
	inst, ok := instanceFromService(project, env, *data.ServiceInstance)
	if !ok {
		return Instance{}, fmt.Errorf("railway service %s was incomplete", serviceID)
	}
	if identity := c.identityPath(); identity != "" {
		inst.IdentityFile = identity
	}
	return inst, nil
}

func FormatSyncID(projectID, environmentID, serviceID string) string {
	return projectID + "/" + environmentID + "/" + serviceID
}

func ParseSyncID(syncID string) (projectID, environmentID, serviceID string, err error) {
	id := strings.TrimSpace(syncID)
	parts := strings.Split(id, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid Railway sync id %q", syncID)
	}
	projectID, environmentID, serviceID = parts[0], parts[1], parts[2]
	for _, part := range []string{projectID, environmentID, serviceID} {
		if part == "" || strings.ContainsAny(part, " \t\r\n\\") {
			return "", "", "", fmt.Errorf("invalid Railway sync id %q", syncID)
		}
		for _, r := range part {
			if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
				return "", "", "", fmt.Errorf("invalid Railway sync id %q", syncID)
			}
		}
	}
	return projectID, environmentID, serviceID, nil
}

func normalizeState(status string, stopped bool) string {
	if stopped {
		return "stopped"
	}
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS":
		return "running"
	case "DEPLOYING", "BUILDING", "QUEUED", "WAITING", "INITIALIZING":
		return "starting"
	case "SLEEPING", "REMOVED", "SKIPPED", "REMOVING":
		return "stopped"
	case "FAILED", "CRASHED":
		return "error"
	case "":
		return "stopped"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isVisibleRunning(state string) bool {
	switch state {
	case "running", "starting":
		return true
	default:
		return false
	}
}

func isReadyState(state string) bool {
	return state == "running"
}

func IsStoppedState(state string) bool {
	switch state {
	case "stopped", "error":
		return true
	default:
		return false
	}
}

func HostLooksStopped(tags []string) bool {
	return IsStoppedState(StateFromTags(tags))
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}
