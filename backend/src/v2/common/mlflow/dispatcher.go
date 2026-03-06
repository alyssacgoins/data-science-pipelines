package mlflow

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	commonmlflow "github.com/kubeflow/pipelines/backend/src/common/mlflow"
	"github.com/kubeflow/pipelines/backend/src/v2/common/plugins"
	"github.com/kubeflow/pipelines/backend/src/v2/config"
	"github.com/kubeflow/pipelines/backend/src/v2/metadata"
	k8score "k8s.io/api/core/v1"
)

var _ plugins.TaskPluginDispatcher = (*TaskPluginDispatcher)(nil)

// Dispatcher implements TaskPluginDispatcher for MLflow.
type TaskPluginDispatcher struct {
	pluginCfg commonmlflow.PluginConfig
	taskInfo  TaskInfo
}

// NewDispatcher creates a new MLflow plugin dispatcher.
func NewDispatcher() (*TaskPluginDispatcher, error) {
	cfg, err := config.FormatKfpMLflowRuntimeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to format MLflow runtime config: %s", err)
	}

	info, err := getTaskInfoFromRuntimeConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to format MLflow TaskInfo: %s", err)
	}
	pluginCfg := getPluginConfigFromRuntimeConfig(cfg)
	return &TaskPluginDispatcher{
		taskInfo:  *info,
		pluginCfg: pluginCfg,
	}, nil
}

func (d *TaskPluginDispatcher) OnTaskStart(ctx context.Context) (*plugins.TaskStartResult, error) {
	handler := NewHandler()
	taskStartResult, err := handler.OnTaskStart(ctx, d.taskInfo, d.pluginCfg)
	if err != nil {
		glog.Errorf("failed to launch task: %s", err)
	} else {
		d.taskInfo.RunID = taskStartResult.RunID
	}
	return taskStartResult, err
}

func (d *TaskPluginDispatcher) OnTaskEnd(ctx context.Context, taskExecution interface{}) error {
	//todo: need to add metrics correctly.
	var metrics map[string]float64
	var params map[string]string

	execution, ok := taskExecution.(*metadata.Execution)
	if !ok {
		glog.Errorf("invalid execution: %v", taskExecution)
	} else {
		exec := execution.GetExecution()

		inputParams, _, err := execution.GetParameters()
		if err != nil {
			return err
		}
		for key, value := range inputParams {
			valueFormatted, err := metadata.PbValueToText(value)
			if err != nil {
				glog.Errorf("Failed to format parameter value for key %s: %v", key, err)
				continue
			}

			params[key] = valueFormatted

		}
		d.taskInfo.RunID = exec.CustomProperties["mlflow_run_id"].GetStringValue()
		d.taskInfo.RunEndTime = exec.GetLastUpdateTimeSinceEpoch()
		d.taskInfo.RunStatus = exec.LastKnownState.String()
	}

	handler := NewHandler()
	err := handler.OnTaskEnd(ctx, d.taskInfo, metrics, params, d.pluginCfg)
	if err != nil {
		return fmt.Errorf("failed to complete task: %s", err)
	}

	return nil
}

// RetrieveUserContainerEnvVars retrieves the env vars that MLflow wants to pass into the user container.
// ToDo: Add test case in driver to valid non-k8s auth failure. Fix all driver tests here.
func (d *TaskPluginDispatcher) RetrieveUserContainerEnvVars() []k8score.EnvVar {
	var injectVars []k8score.EnvVar
	// only inject env vars if injection is enabled
	if d.taskInfo.InjectUserEnvVars {
		injectVars = []k8score.EnvVar{
			{Name: "MLFLOW_RUN_ID", Value: d.taskInfo.RunID},
			{Name: "MLFLOW_TRACKING_URI", Value: d.pluginCfg.Endpoint},
			{Name: "MLFLOW_EXPERIMENT_ID", Value: d.taskInfo.ExperimentID},
		}

		// set MLFLOW_TRACKING_AUTH only if auth type is "kubernetes". if MLflow workspaces are enabled, use "kubernetes-namespaced"
		var auth string
		if d.taskInfo.AuthType == "kubernetes" {
			auth = "kubernetes"
			if *d.pluginCfg.Settings.WorkspacesEnabled {
				auth = "kubernetes-namespaced"
				injectVars = append(injectVars, k8score.EnvVar{Name: "MLFLOW_WORKSPACE", Value: d.taskInfo.Workspace})
			}
			injectVars = append(injectVars, k8score.EnvVar{Name: "MLFLOW_TRACKING_AUTH", Value: auth})
		} else {
			glog.Warningf("MLflow auth type %s is not supported", d.taskInfo.AuthType)
		}
	}
	return injectVars
}

func getTaskInfoFromRuntimeConfig(runtimeCfg commonmlflow.MLflowRuntimeConfig) (*TaskInfo, error) {
	if runtimeCfg.ParentRunID == "" {
		return nil, fmt.Errorf("ParentRunID is required to create MLflow task")
	}
	if runtimeCfg.ExperimentID == "" {
		return nil, fmt.Errorf("ExperimentID is required to create MLflow task")
	}
	if runtimeCfg.AuthType == "" {
		return nil, fmt.Errorf("AuthType is required to create MLflow task")
	}

	return &TaskInfo{
		Workspace:         runtimeCfg.Workspace,
		WorkspacesEnabled: runtimeCfg.Workspace != "",
		ParentRunID:       runtimeCfg.ParentRunID,
		ExperimentID:      runtimeCfg.ExperimentID,
		AuthType:          runtimeCfg.AuthType,
		InjectUserEnvVars: runtimeCfg.InjectUserEnvVars,
	}, nil
}

func getPluginConfigFromRuntimeConfig(runtimeCfg commonmlflow.MLflowRuntimeConfig) commonmlflow.PluginConfig {
	tlsCfg := commonmlflow.TLSConfig{
		InsecureSkipVerify: runtimeCfg.InsecureSkipVerify,
	}
	return commonmlflow.PluginConfig{
		Endpoint: runtimeCfg.Endpoint,
		Timeout:  runtimeCfg.Timeout,
		TLS:      &tlsCfg,
	}
}
