// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/copilot-cli/internal/pkg/manifest"
	"github.com/google/uuid"
)

// Paths of workload cloudformation templates under templates/workloads/.
const (
	fmtWkldCFTemplatePath         = "workloads/%s/%s/cf.yml"
	fmtWkldPartialsCFTemplatePath = "workloads/partials/cf/%s.yml"
)

const (
	servicesDirName = "services"
	jobDirName      = "jobs"
)

var (
	// Template names under "workloads/partials/cf/".
	partialsWorkloadCFTemplateNames = []string{
		"loggroup",
		"envvars",
		"secrets",
		"executionrole",
		"taskrole",
		"fargate-taskdef-base-properties",
		"service-base-properties",
		"servicediscovery",
		"addons",
		"sidecars",
		"logconfig",
		"autoscaling",
		"eventrule",
		"state-machine",
		"state-machine-definition.json",
		"env-controller",
		"mount-points",
		"volumes",
	}
)

// Names of workload templates.
const (
	lbWebSvcTplName     = "lb-web"
	backendSvcTplName   = "backend"
	scheduledJobTplName = "scheduled-job"
)

// Validation errors when rendering manifest into template.
var (
	errNoFSID          = errors.New("volume field efs/id cannot be empty")
	errNoContainerPath = errors.New("volume field path cannot be empty")
)

var (
	pEnabled  = aws.String("ENABLED")
	pDisabled = aws.String("DISABLED")
)

// Default values for EFS options
var (
	defaultRootDirectory   = aws.String("/")
	defaultIAM             = pDisabled
	defaultReadOnly        = aws.Bool(true)
	defaultWritePermission = false
)

// WorkloadNestedStackOpts holds configuration that's needed if the workload stack has a nested stack.
type WorkloadNestedStackOpts struct {
	StackName string

	VariableOutputs []string
	SecretOutputs   []string
	PolicyOutputs   []string
}

// SidecarOpts holds configuration that's needed if the service has sidecar containers.
type SidecarOpts struct {
	Name        *string
	Image       *string
	Port        *string
	Protocol    *string
	CredsParam  *string
	Variables   map[string]string
	Secrets     map[string]string
	MountPoints []*MountPoint
}

// StorageOpts holds data structures for rendering Volumes and Mount Points
type StorageOpts struct {
	Volumes     []*Volume
	MountPoints []*MountPoint
	EFSPerms    []*EFSPermission
}

// RenderStorageOpts converts a manifest.Storage field into template data structures which can be used
// to execute CFN templates
func RenderStorageOpts(in manifest.Storage) (*StorageOpts, error) {
	v, err := renderVolumes(in.Volumes)
	if err != nil {
		return nil, err
	}
	mp, err := renderMountPoints(in.Volumes)
	if err != nil {
		return nil, err
	}
	perms, err := renderStoragePermissions(in.Volumes)
	if err != nil {
		return nil, err
	}
	return &StorageOpts{
		Volumes:     v,
		MountPoints: mp,
		EFSPerms:    perms,
	}, nil
}

// RenderSidecarMountPoints is used to convert from manifest to template objects.
func RenderSidecarMountPoints(in []manifest.SidecarMountPoint) []*MountPoint {
	if len(in) == 0 {
		return nil
	}
	output := []*MountPoint{}
	for _, smp := range in {
		mp := MountPoint{
			ContainerPath: smp.ContainerPath,
			SourceVolume:  smp.SourceVolume,
			ReadOnly:      smp.ReadOnly,
		}
		output = append(output, &mp)
	}
	return output
}

func renderStoragePermissions(input map[string]manifest.Volume) ([]*EFSPermission, error) {
	if len(input) == 0 {
		return nil, nil
	}
	output := []*EFSPermission{}
	for _, volume := range input {
		// Write defaults to false
		write := defaultWritePermission
		if volume.ReadOnly != nil {
			write = !aws.BoolValue(volume.ReadOnly)
		}
		if volume.EFS.FileSystemID == nil {
			return nil, errNoFSID
		}
		perm := EFSPermission{
			Write:         write,
			AccessPointID: volume.EFS.AuthConfig.AccessPointID,
			FilesystemID:  volume.EFS.FileSystemID,
		}
		output = append(output, &perm)
	}
	return output, nil
}

func renderMountPoints(input map[string]manifest.Volume) ([]*MountPoint, error) {
	if len(input) == 0 {
		return nil, nil
	}
	output := []*MountPoint{}
	for name, volume := range input {
		// ContainerPath must be specified.
		if volume.ContainerPath == nil {
			return nil, errNoContainerPath
		}
		// ReadOnly defaults to true.
		readOnly := defaultReadOnly
		if volume.ReadOnly != nil {
			readOnly = volume.ReadOnly
		}
		mp := MountPoint{
			ReadOnly:      readOnly,
			ContainerPath: volume.ContainerPath,
			SourceVolume:  aws.String(name),
		}
		output = append(output, &mp)
	}
	return output, nil
}

func renderVolumes(input map[string]manifest.Volume) ([]*Volume, error) {
	if len(input) == 0 {
		return nil, nil
	}
	output := []*Volume{}
	for name, volume := range input {
		// Set default values correctly.
		fsID := volume.EFS.FileSystemID
		if aws.StringValue(fsID) == "" {
			return nil, errNoFSID
		}
		rootDir := volume.EFS.RootDirectory
		if aws.StringValue(rootDir) == "" {
			rootDir = defaultRootDirectory
		}
		var iam *string
		if volume.EFS.AuthConfig.IAM == nil {
			iam = defaultIAM
		}
		if aws.BoolValue(volume.EFS.AuthConfig.IAM) {
			iam = pEnabled
		}
		v := Volume{
			Name: aws.String(name),

			Filesystem:    fsID,
			RootDirectory: rootDir,

			AccessPointID: volume.EFS.AuthConfig.AccessPointID,
			IAM:           iam,
		}
		output = append(output, &v)
	}
	return output, nil
}

// EFSPermission holds information needed to render an IAM policy statement.
type EFSPermission struct {
	FilesystemID  *string
	Write         bool
	AccessPointID *string
}

// MountPoint holds information needed to render a MountPoint in a containerdefinition.
type MountPoint struct {
	ContainerPath *string
	ReadOnly      *bool
	SourceVolume  *string
}

// Volume contains fields that render a volume, its name, and EFSVolumeConfiguration
type Volume struct {
	Name *string

	// EFSVolumeConfiguration
	Filesystem    *string
	RootDirectory *string // "/" or empty are equivalent

	// Authorization Config
	AccessPointID *string
	IAM           *string // ENABLED or DISABLED
}

// LogConfigOpts holds configuration that's needed if the service is configured with Firelens to route
// its logs.
type LogConfigOpts struct {
	Image          *string
	Destination    map[string]string
	EnableMetadata *string
	SecretOptions  map[string]string
	ConfigFile     *string
}

// HTTPHealthCheckOpts holds configuration that's needed for HTTP Health Check.
type HTTPHealthCheckOpts struct {
	HealthCheckPath    string
	HealthyThreshold   *int64
	UnhealthyThreshold *int64
	Interval           *int64
	Timeout            *int64
}

// AutoscalingOpts holds configuration that's needed for Auto Scaling.
type AutoscalingOpts struct {
	MinCapacity  *int
	MaxCapacity  *int
	CPU          *float64
	Memory       *float64
	Requests     *float64
	ResponseTime *float64
}

// StateMachineOpts holds configuration neeed for State Machine retries and timeout.
type StateMachineOpts struct {
	Timeout *int
	Retries *int
}

// WorkloadOpts holds optional data that can be provided to enable features in a workload stack template.
type WorkloadOpts struct {
	// Additional options that are common between **all** workload templates.
	Variables   map[string]string
	Secrets     map[string]string
	NestedStack *WorkloadNestedStackOpts // Outputs from nested stacks such as the addons stack.
	Sidecars    []*SidecarOpts
	LogConfig   *LogConfigOpts
	Autoscaling *AutoscalingOpts
	Storage     StorageOpts

	// Additional options for service templates.
	HealthCheck         *ecs.HealthCheck
	HTTPHealthCheck     HTTPHealthCheckOpts
	AllowedSourceIps    []string
	RulePriorityLambda  string
	DesiredCountLambda  string
	EnvControllerLambda string

	// Additional options for job templates.
	ScheduleExpression string
	StateMachine       *StateMachineOpts
}

// ParseLoadBalancedWebService parses a load balanced web service's CloudFormation template
// with the specified data object and returns its content.
func (t *Template) ParseLoadBalancedWebService(data WorkloadOpts) (*Content, error) {
	return t.parseSvc(lbWebSvcTplName, data, withSvcParsingFuncs())
}

// ParseBackendService parses a backend service's CloudFormation template with the specified data object and returns its content.
func (t *Template) ParseBackendService(data WorkloadOpts) (*Content, error) {
	return t.parseSvc(backendSvcTplName, data, withSvcParsingFuncs())
}

// ParseScheduledJob parses a scheduled job's Cloudformation Template
func (t *Template) ParseScheduledJob(data WorkloadOpts) (*Content, error) {
	return t.parseJob(scheduledJobTplName, data, withSvcParsingFuncs())
}

// parseSvc parses a service's CloudFormation template with the specified data object and returns its content.
func (t *Template) parseSvc(name string, data interface{}, options ...ParseOption) (*Content, error) {
	return t.parseWkld(name, servicesDirName, data, options...)
}

// parseJob parses a job's Cloudformation template with the specified data object and returns its content.
func (t *Template) parseJob(name string, data interface{}, options ...ParseOption) (*Content, error) {
	return t.parseWkld(name, jobDirName, data, options...)
}

func (t *Template) parseWkld(name, wkldDirName string, data interface{}, options ...ParseOption) (*Content, error) {
	tpl, err := t.parse("base", fmt.Sprintf(fmtWkldCFTemplatePath, wkldDirName, name), options...)
	if err != nil {
		return nil, err
	}
	for _, templateName := range partialsWorkloadCFTemplateNames {
		nestedTpl, err := t.parse(templateName, fmt.Sprintf(fmtWkldPartialsCFTemplatePath, templateName), options...)
		if err != nil {
			return nil, err
		}
		_, err = tpl.AddParseTree(templateName, nestedTpl.Tree)
		if err != nil {
			return nil, fmt.Errorf("add parse tree of %s to base template: %w", templateName, err)
		}
	}
	buf := &bytes.Buffer{}
	if err := tpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("execute template %s with data %v: %w", name, data, err)
	}
	return &Content{buf}, nil
}

func withSvcParsingFuncs() ParseOption {
	return func(t *template.Template) *template.Template {
		return t.Funcs(map[string]interface{}{
			"toSnakeCase": ToSnakeCaseFunc,
			"hasSecrets":  hasSecrets,
			"fmtSlice":    FmtSliceFunc,
			"quoteSlice":  QuotePSliceFunc,
			"randomUUID":  randomUUIDFunc,
		})
	}
}

func hasSecrets(opts WorkloadOpts) bool {
	if len(opts.Secrets) > 0 {
		return true
	}
	if opts.NestedStack != nil && (len(opts.NestedStack.SecretOutputs) > 0) {
		return true
	}
	return false
}

func randomUUIDFunc() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate random uuid: %w", err)
	}
	return id.String(), err
}
