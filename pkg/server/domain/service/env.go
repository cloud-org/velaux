/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sort"

	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/oam"
	util "github.com/oam-dev/kubevela/pkg/utils"

	"github.com/kubevela/velaux/pkg/server/domain/model"
	"github.com/kubevela/velaux/pkg/server/domain/repository"
	"github.com/kubevela/velaux/pkg/server/infrastructure/datastore"
	apisv1 "github.com/kubevela/velaux/pkg/server/interfaces/api/dto/v1"
	"github.com/kubevela/velaux/pkg/server/utils"
	"github.com/kubevela/velaux/pkg/server/utils/bcode"
)

// EnvService defines the API of Env.
type EnvService interface {
	GetEnv(ctx context.Context, envName string) (*model.Env, error)
	ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error)
	ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error)
	DeleteEnv(ctx context.Context, envName string) error
	CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error)
	UpdateEnv(ctx context.Context, envName string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error)
}

type envServiceImpl struct {
	Store          datastore.DataStore `inject:"datastore"`
	ProjectService ProjectService      `inject:""`
	KubeClient     client.Client       `inject:"kubeClient"`
}

// NewEnvService new env service
func NewEnvService() EnvService {
	return &envServiceImpl{}
}

// GetEnv get env
func (p *envServiceImpl) GetEnv(ctx context.Context, envName string) (*model.Env, error) {
	return repository.GetEnv(ctx, p.Store, envName)
}

// DeleteEnv delete an env by name
// the function assume applications contain in env already empty.
// it won't delete the namespace created by the Env, but it will update the label
func (p *envServiceImpl) DeleteEnv(ctx context.Context, envName string) error {
	env := &model.Env{}
	env.Name = envName

	if err := p.Store.Get(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}
	// reset the labels
	err := util.UpdateNamespace(ctx, p.KubeClient, env.Namespace, util.MergeOverrideLabels(map[string]string{
		oam.LabelNamespaceOfEnvName:         "",
		oam.LabelControlPlaneNamespaceUsage: "",
	}))
	if err != nil && apierror.IsNotFound(err) {
		return err
	}

	if err = p.Store.Delete(ctx, env); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil
		}
		return err
	}

	if err := managePrivilegesForEnvironment(ctx, p.KubeClient, env, true); err != nil {
		return err
	}

	return nil
}

// ListEnvs list envs
func (p *envServiceImpl) ListEnvs(ctx context.Context, page, pageSize int, listOption apisv1.ListEnvOptions) (*apisv1.ListEnvResponse, error) {
	userName, ok := ctx.Value(&apisv1.CtxKeyUser).(string)
	if !ok {
		return nil, bcode.ErrUnauthorized
	}
	projects, err := p.ProjectService.ListUserProjects(ctx, userName)
	if err != nil {
		return nil, err
	}
	var availableProjectNames []string
	var projectNameAlias = make(map[string]string)
	for _, project := range projects {
		availableProjectNames = append(availableProjectNames, project.Name)
		projectNameAlias[project.Name] = project.Alias
	}
	if len(availableProjectNames) == 0 {
		return &apisv1.ListEnvResponse{Envs: []*apisv1.Env{}, Total: 0}, nil
	}
	if listOption.Project != "" {
		if !util.StringsContain(availableProjectNames, listOption.Project) {
			return &apisv1.ListEnvResponse{Envs: []*apisv1.Env{}, Total: 0}, nil
		}
	}
	projectNames := []string{listOption.Project}
	if listOption.Project == "" {
		projectNames = availableProjectNames
	}
	filter := datastore.FilterOptions{
		In: []datastore.InQueryOption{
			{
				Key:    "project",
				Values: projectNames,
			},
		},
	}
	entities, err := repository.ListEnvs(ctx, p.Store, &datastore.ListOptions{
		Page:          page,
		PageSize:      pageSize,
		SortBy:        []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
		FilterOptions: filter,
	})
	if err != nil {
		return nil, err
	}

	targets, err := repository.ListTarget(ctx, p.Store, listOption.Project, nil)
	if err != nil {
		return nil, err
	}

	var envs []*apisv1.Env
	for _, ee := range entities {
		envs = append(envs, convertEnvModel2Base(ee, targets))
	}

	for i := range envs {
		envs[i].Project.Alias = projectNameAlias[envs[i].Project.Name]
	}

	total, err := p.Store.Count(ctx, &model.Env{Project: listOption.Project}, &filter)
	if err != nil {
		return nil, err
	}
	return &apisv1.ListEnvResponse{Envs: envs, Total: total}, nil
}

func (p *envServiceImpl) ListEnvCount(ctx context.Context, listOption apisv1.ListEnvOptions) (int64, error) {
	return p.Store.Count(ctx, &model.Env{Project: listOption.Project}, nil)
}

func checkEqual(old, new []string) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}
	sort.Strings(old)
	sort.Strings(new)
	return reflect.DeepEqual(old, new)
}

// UpdateEnv update an env for request
func (p *envServiceImpl) UpdateEnv(ctx context.Context, name string, req apisv1.UpdateEnvRequest) (*apisv1.Env, error) {
	env := &model.Env{}
	env.Name = name
	err := p.Store.Get(ctx, env)
	if err != nil {
		klog.Errorf("check if env name exists failure %s", err.Error())
		return nil, bcode.ErrEnvNotExisted
	}
	if req.Alias != "" {
		env.Alias = req.Alias
	}
	if req.Description != "" {
		env.Description = req.Description
	}

	pass, err := p.checkEnvTarget(ctx, env.Project, env.Name, req.Targets)
	if err != nil || !pass {
		return nil, bcode.ErrEnvTargetConflict
	}
	var targets []*model.Target
	if len(req.Targets) > 0 {
		_, _, deleted := util.ThreeWaySliceCompare(req.Targets, env.Targets)
		if len(deleted) > 0 {
			count, err := p.GetAppCountInEnv(ctx, env)
			if err != nil {
				return nil, err
			}
			if count > 0 {
				return nil, bcode.ErrEnvTargetNotAllowDelete
			}
		}
		targets, err = repository.ListTarget(ctx, p.Store, "", &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{{
					Key:    "name",
					Values: req.Targets,
				}},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(targets) != len(req.Targets) {
			return nil, bcode.ErrTargetNotExist
		}
		env.Targets = req.Targets
	}

	// create namespace at first
	if err := p.Store.Put(ctx, env); err != nil {
		return nil, err
	}

	// Updating the role and role binding can't use the login user permissions.
	updateRoleCtx := utils.WithProject(ctx, "")
	if err := managePrivilegesForEnvironment(updateRoleCtx, p.KubeClient, env, false); err != nil {
		return nil, err
	}

	resp := convertEnvModel2Base(env, targets)
	return resp, nil
}

func (p *envServiceImpl) GetAppCountInEnv(ctx context.Context, env *model.Env) (int, error) {
	var appList v1beta1.ApplicationList
	if err := p.KubeClient.List(ctx, &appList, client.InNamespace(env.Namespace), client.MatchingLabels{types.LabelSourceOfTruth: types.FromUX}); err != nil {
		return 0, err
	}
	return len(appList.Items), nil
}

// CreateEnv create an env for request
func (p *envServiceImpl) CreateEnv(ctx context.Context, req apisv1.CreateEnvRequest) (*apisv1.Env, error) {
	newEnv := &model.Env{
		Name:        req.Name,
		Alias:       req.Alias,
		Description: req.Description,
		Namespace:   req.Namespace,
		Project:     req.Project,
		Targets:     req.Targets,
	}

	if !req.AllowTargetConflict {
		pass, err := p.checkEnvTarget(ctx, req.Project, req.Name, req.Targets)
		if err != nil || !pass {
			return nil, bcode.ErrEnvTargetConflict
		}
	}

	targets, err := repository.ListTarget(ctx, p.Store, "", nil)
	if err != nil {
		return nil, err
	}

	var targetMap = make(map[string]*model.Target, len(targets))
	for i, existTarget := range targets {
		targetMap[existTarget.Name] = targets[i]
	}

	for _, target := range req.Targets {
		if _, exist := targetMap[target]; !exist {
			return nil, bcode.ErrTargetNotExist
		}
	}

	// Creating the namespace can't use the login user permissions.
	createNamespaceCtx := utils.WithProject(ctx, "")
	err = repository.CreateEnv(createNamespaceCtx, p.KubeClient, p.Store, newEnv)
	if err != nil {
		return nil, err
	}

	if err := managePrivilegesForEnvironment(createNamespaceCtx, p.KubeClient, newEnv, false); err != nil {
		return nil, err
	}

	resp := convertEnvModel2Base(newEnv, targets)
	return resp, nil
}

// checkEnvTarget In one project, a delivery target can only belong to one env.
func (p *envServiceImpl) checkEnvTarget(ctx context.Context, project string, envName string, targets []string) (bool, error) {
	if len(targets) == 0 {
		return true, nil
	}
	entities, err := p.Store.List(ctx, &model.Env{Project: project}, &datastore.ListOptions{})
	if err != nil {
		return false, err
	}
	newMap := make(map[string]bool, len(targets))
	for _, new := range targets {
		newMap[new] = true
	}
	for _, entity := range entities {
		env := entity.(*model.Env)
		for _, existTarget := range env.Targets {
			if ok := newMap[existTarget]; ok && env.Name != envName {
				return false, nil
			}
		}
	}
	return true, nil
}

func convertEnvModel2Base(env *model.Env, targets []*model.Target) *apisv1.Env {
	data := apisv1.Env{
		Name:        env.Name,
		Alias:       env.Alias,
		Description: env.Description,
		Project:     apisv1.NameAlias{Name: env.Project},
		Namespace:   env.Namespace,
		CreateTime:  env.CreateTime,
		UpdateTime:  env.UpdateTime,
	}
	for _, dt := range env.Targets {
		var t *model.Target
		for _, tg := range targets {
			if dt == tg.Name {
				t = tg
				break
			}
		}
		if t != nil {
			data.Targets = append(data.Targets, apisv1.NameAlias{
				Name:  dt,
				Alias: t.Alias,
			})
		} else {
			data.Targets = append(data.Targets, apisv1.NameAlias{
				Name: dt,
			})
		}
	}
	return &data
}

// managePrivilegesForEnvironment grant or revoke privileges for environment
func managePrivilegesForEnvironment(ctx context.Context, cli client.Client, env *model.Env, revoke bool) error {
	p := &auth.ApplicationPrivilege{Cluster: types.ClusterLocalName, Namespace: env.Namespace}
	identity := &auth.Identity{Groups: []string{utils.KubeVelaProjectGroupPrefix + env.Project}}
	writer := &bytes.Buffer{}
	f, msg := auth.GrantPrivileges, "GrantPrivileges"
	if revoke {
		f, msg = auth.RevokePrivileges, "RevokePrivileges"
	}
	if err := f(ctx, cli, []auth.PrivilegeDescription{p}, identity, writer); err != nil {
		return err
	}
	klog.Infof("%s: %s", msg, writer.String())
	return nil
}

// NewTestEnvService create the env service instance for testing
func NewTestEnvService(ds datastore.DataStore, c client.Client) EnvService {
	return &envServiceImpl{Store: ds, KubeClient: c, ProjectService: NewTestProjectService(ds, c)}
}
