package flink

import (
	"context"
	"testing"

	"github.com/lyft/flinkk8soperator/pkg/apis/app/v1alpha1"
	clientMock "github.com/lyft/flinkk8soperator/pkg/controller/flink/client/mock"
	"github.com/lyft/flinkk8soperator/pkg/controller/flink/mock"
	k8mock "github.com/lyft/flinkk8soperator/pkg/controller/k8/mock"
	mockScope "github.com/lyft/flytestdlib/promutils"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/apps/v1"

	"github.com/lyft/flinkk8soperator/pkg/controller/common"
	"github.com/lyft/flinkk8soperator/pkg/controller/flink/client"
	"github.com/lyft/flytestdlib/promutils/labeled"
	"github.com/pkg/errors"
)

const testImage = "123.xyz.com/xx:11ae1218924428faabd9b64423fa0c332efba6b2"
const testImageKey = "11ae1"
const testAppName = "app-name"
const testNamespace = "ns"
const testJobId = "j1"

func getTestFlinkController() FlinkController {
	testScope := mockScope.NewTestScope()
	labeled.SetMetricKeys(common.GetValidLabelNames()...)
	return FlinkController{
		flinkJobManager:  &mock.MockJobManagerController{},
		FlinkTaskManager: &mock.MockTaskManagerController{},
		k8Cluster:        &k8mock.MockK8Cluster{},
		flinkClient:      &clientMock.MockJobManagerClient{},
		metrics:          newFlinkControllerMetrics(testScope),
	}
}

func getFlinkTestApp() v1alpha1.FlinkApplication {
	app := v1alpha1.FlinkApplication{}
	app.Name = testAppName
	app.Namespace = testNamespace
	app.Status.JobId = testJobId
	app.Spec.Image = testImage

	return app
}

func TestFlinkIsClusterReady(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	labelMapVal := map[string]string{
		"imageKey": testImageKey,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.IsAllPodsRunningFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (bool, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return true, nil
	}
	flinkApp := getFlinkTestApp()
	result, err := flinkControllerForTest.IsClusterReady(
		context.Background(), &flinkApp,
	)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestFlinkIsClusterChangeNeeded(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	labelMapVal := map[string]string{
		"imageKey": testImageKey,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				{
					Status: v1.DeploymentStatus{
						Replicas: 2,
					},
				},
			},
		}, nil
	}
	flinkApp := getFlinkTestApp()
	result, err := flinkControllerForTest.IsClusterChangeNeeded(
		context.Background(), &flinkApp,
	)
	assert.False(t, result)
	assert.Nil(t, err)
}

func TestFlinkIsClusterChangeNotNeeded(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	labelMapVal := map[string]string{
		"imageKey": testImageKey,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return &v1.DeploymentList{}, nil
	}
	flinkApp := getFlinkTestApp()
	result, err := flinkControllerForTest.IsClusterChangeNeeded(
		context.Background(), &flinkApp,
	)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestFlinkApplicationChanged(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	labelMapVal := map[string]string{
		"imageKey": testImageKey,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return &v1.DeploymentList{}, nil
	}
	flinkApp := getFlinkTestApp()
	result, err := flinkControllerForTest.HasApplicationChanged(
		context.Background(), &flinkApp,
	)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestFlinkApplicationNeededNeedUpdate(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	numberOfTaskManagers := int32(2)
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		if val, ok := labelMap["imageKey"]; ok {
			assert.Equal(t, testImageKey, val)
		}
		if val, ok := labelMap["App"]; ok {
			assert.Equal(t, testAppName, val)
		}
		deployment := v1.Deployment{
			Spec: v1.DeploymentSpec{
				Replicas: &numberOfTaskManagers,
			},
		}
		deployment.Name = testAppName + "-" + testImageKey + "-tm"
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				deployment,
			},
		}, nil
	}
	flinkApp := getFlinkTestApp()
	taskSlots := int32(2)
	flinkApp.Spec.TaskManagerConfig.TaskSlots = &taskSlots
	flinkApp.Spec.FlinkJob.Parallelism = taskSlots*numberOfTaskManagers + 1
	result, err := flinkControllerForTest.HasApplicationChanged(
		context.Background(), &flinkApp,
	)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestFlinkApplicationParallelismChanged(t *testing.T) {
	taskSlots := int32(1)
	flinkControllerForTest := getTestFlinkController()
	mockJMClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJMClient.GetJobConfigFunc = func(ctx context.Context, url string, jobId string) (*client.JobConfigResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jobId, testJobId)

		return &client.JobConfigResponse{
			ExecutionConfig: client.JobExecutionConfig{
				Parallelism: 3,
			},
		}, nil
	}
	flinkApp := getFlinkTestApp()
	flinkApp.Spec.FlinkJob.Parallelism = 2
	flinkApp.Spec.TaskManagerConfig.TaskSlots = &taskSlots

	result, err := flinkControllerForTest.HasApplicationJobChanged(
		context.Background(), &flinkApp,
	)
	assert.True(t, result)
	assert.Nil(t, err)
}

func TestFlinkApplicationJobConfigErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		deployment := v1.Deployment{}
		deployment.Name = getJobManagerName(&flinkApp)
		replicas := int32(1)
		deployment.Spec.Replicas = &replicas
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				deployment,
			},
		}, nil
	}

	mockJMClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJMClient.GetJobConfigFunc = func(ctx context.Context, url string, jobId string) (*client.JobConfigResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jobId, testJobId)
		return nil, errors.New("JobConfig Failure")
	}
	result, err := flinkControllerForTest.HasApplicationChanged(
		context.Background(), &flinkApp,
	)
	assert.False(t, result)
	assert.EqualError(t, err, "JobConfig Failure")
}

func TestFlinkIsServiceReady(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.GetClusterOverviewFunc = func(ctx context.Context, url string) (*client.ClusterOverviewResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		return &client.ClusterOverviewResponse{
			TaskManagerCount: 3,
		}, nil
	}
	isReady, err := flinkControllerForTest.IsServiceReady(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.True(t, isReady)
}

func TestFlinkIsServiceReadyErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.GetClusterOverviewFunc = func(ctx context.Context, url string) (*client.ClusterOverviewResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		return nil, errors.New("Get cluster failed")
	}
	isReady, err := flinkControllerForTest.IsServiceReady(context.Background(), &flinkApp)
	assert.EqualError(t, err, "Get cluster failed")
	assert.False(t, isReady)
}

func TestFlinkGetSavepointStatus(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	flinkApp.Spec.FlinkJob.SavepointInfo.TriggerId = "t1"

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.CheckSavepointStatusFunc = func(ctx context.Context, url string, jobId, triggerId string) (*client.SavepointResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jobId, testJobId)
		assert.Equal(t, triggerId, "t1")
		return &client.SavepointResponse{
			SavepointStatus: client.SavepointStatusResponse{
				Status: client.SavePointInProgress,
			},
		}, nil
	}
	status, err := flinkControllerForTest.GetSavepointStatus(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.NotNil(t, status)

	assert.Equal(t, client.SavePointInProgress, status.SavepointStatus.Status)
}

func TestFlinkGetSavepointStatusErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.CheckSavepointStatusFunc = func(ctx context.Context, url string, jobId, triggerId string) (*client.SavepointResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jobId, testJobId)
		return nil, errors.New("Savepoint error")
	}
	status, err := flinkControllerForTest.GetSavepointStatus(context.Background(), &flinkApp)
	assert.Nil(t, status)
	assert.NotNil(t, err)

	assert.EqualError(t, err, "Savepoint error")
}

func TestGetActiveJob(t *testing.T) {
	job := client.FlinkJob{
		Status: client.FlinkJobRunning,
		JobId:  "j1",
	}
	jobs := []client.FlinkJob{
		job,
	}
	activeJob := GetActiveFlinkJob(jobs)
	assert.NotNil(t, activeJob)
	assert.Equal(t, *activeJob, job)
}

func TestGetActiveJobNil(t *testing.T) {
	job := client.FlinkJob{
		Status: client.FlinkJobCancelling,
		JobId:  "j1",
	}
	jobs := []client.FlinkJob{
		job,
	}
	activeJob := GetActiveFlinkJob(jobs)
	assert.Nil(t, activeJob)
}

func TestGetActiveJobEmpty(t *testing.T) {
	jobs := []client.FlinkJob{}
	activeJob := GetActiveFlinkJob(jobs)
	assert.Nil(t, activeJob)
}

func TestIsMultipleClusterPresentTrue(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}

	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)

		d1 := v1.Deployment{}
		d1.Labels = labelMapVal
		d2 := v1.Deployment{}
		d2.Labels = map[string]string{
			"imageKey": testImageKey,
		}
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				d1, d2,
			},
		}, nil
	}
	isPresent, err := flinkControllerForTest.IsMultipleClusterPresent(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.True(t, isPresent)
}

func TestIsMultipleClusterPresentFalse(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}

	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)

		d := v1.Deployment{}
		d.Labels = labelMapVal
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				d, d,
			},
		}, nil
	}
	isPresent, err := flinkControllerForTest.IsMultipleClusterPresent(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.False(t, isPresent)
}

func TestDeleteOldCluster(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}
	d1 := v1.Deployment{}
	d1.Labels = map[string]string{
		"app":      testAppName,
		"imageKey": testImageKey,
	}
	d2 := v1.Deployment{}
	d2.Labels = map[string]string{
		"app":      testAppName,
		"imageKey": testImageKey + "3",
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)

		return &v1.DeploymentList{
			Items: []v1.Deployment{
				d1, d2,
			},
		}, nil
	}
	mockK8Cluster.DeleteDeploymentsFunc = func(ctx context.Context, deploymentList v1.DeploymentList) error {
		assert.Equal(t, v1.DeploymentList{Items: []v1.Deployment{d2}}, deploymentList)
		return nil
	}
	isDeleted, err := flinkControllerForTest.DeleteOldCluster(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.True(t, isDeleted)
}

func TestDeleteOldClusterNoOldDeployment(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		d1 := v1.Deployment{}
		d1.Labels = labelMapVal
		d1.Labels["imageKey"] = testImageKey

		return &v1.DeploymentList{Items: []v1.Deployment{
			d1,
		}}, nil
	}
	mockK8Cluster.DeleteDeploymentsFunc = func(ctx context.Context, deploymentList v1.DeploymentList) error {
		assert.False(t, true)
		return nil
	}
	isDeleted, err := flinkControllerForTest.DeleteOldCluster(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.True(t, isDeleted)
}

func TestDeleteOldClusterNoDeployment(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}
	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return &v1.DeploymentList{}, nil
	}
	mockK8Cluster.DeleteDeploymentsFunc = func(ctx context.Context, deploymentList v1.DeploymentList) error {
		assert.False(t, true)
		return nil
	}
	isDeleted, err := flinkControllerForTest.DeleteOldCluster(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.True(t, isDeleted)
}

func TestDeleteOldClusterErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	labelMapVal := map[string]string{
		"app": testAppName,
	}
	d1 := v1.Deployment{}
	d1.Labels = labelMapVal

	mockK8Cluster := flinkControllerForTest.k8Cluster.(*k8mock.MockK8Cluster)
	mockK8Cluster.GetDeploymentsWithLabelFunc = func(ctx context.Context, namespace string, labelMap map[string]string) (*v1.DeploymentList, error) {
		assert.Equal(t, testNamespace, namespace)
		assert.Equal(t, labelMapVal, labelMap)
		return &v1.DeploymentList{
			Items: []v1.Deployment{
				d1,
			},
		}, nil
	}
	mockK8Cluster.DeleteDeploymentsFunc = func(ctx context.Context, deploymentList v1.DeploymentList) error {
		assert.Equal(t, v1.DeploymentList{Items: []v1.Deployment{d1}}, deploymentList)
		return errors.New("Delete error")
	}
	isDeleted, err := flinkControllerForTest.DeleteOldCluster(context.Background(), &flinkApp)
	assert.EqualError(t, err, "Delete error")
	assert.False(t, isDeleted)
}

func TestCreateCluster(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	mockJobManager := flinkControllerForTest.flinkJobManager.(*mock.MockJobManagerController)
	mockTaskManager := flinkControllerForTest.FlinkTaskManager.(*mock.MockTaskManagerController)

	mockJobManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		return nil
	}
	mockTaskManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		return nil
	}
	err := flinkControllerForTest.CreateCluster(context.Background(), &flinkApp)
	assert.Nil(t, err)
}

func TestCreateClusterJmErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	mockJobManager := flinkControllerForTest.flinkJobManager.(*mock.MockJobManagerController)
	mockTaskManager := flinkControllerForTest.FlinkTaskManager.(*mock.MockTaskManagerController)

	mockJobManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		return errors.New("jm failed")
	}
	mockTaskManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		assert.False(t, true)
		return nil
	}
	err := flinkControllerForTest.CreateCluster(context.Background(), &flinkApp)
	assert.EqualError(t, err, "jm failed")
}

func TestCreateClusterTmErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	mockJobManager := flinkControllerForTest.flinkJobManager.(*mock.MockJobManagerController)
	mockTaskManager := flinkControllerForTest.FlinkTaskManager.(*mock.MockTaskManagerController)

	mockJobManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		return nil
	}
	mockTaskManager.CreateIfNotExistFunc = func(ctx context.Context, application *v1alpha1.FlinkApplication) error {
		return errors.New("tm failed")
	}
	err := flinkControllerForTest.CreateCluster(context.Background(), &flinkApp)
	assert.EqualError(t, err, "tm failed")
}

func TestStartFlinkJob(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()
	flinkApp.Spec.FlinkJob.Parallelism = 4
	flinkApp.Spec.FlinkJob.ProgramArgs = "args"
	flinkApp.Spec.FlinkJob.EntryClass = "class"
	flinkApp.Spec.FlinkJob.JarName = "jar-name"
	flinkApp.Spec.FlinkJob.SavepointInfo.SavepointLocation = "location//"

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.SubmitJobFunc = func(ctx context.Context, url string, jarId string, submitJobRequest client.SubmitJobRequest) (*client.SubmitJobResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jarId, "jar-name")
		assert.Equal(t, submitJobRequest.Parallelism, int32(4))
		assert.Equal(t, submitJobRequest.ProgramArgs, "args")
		assert.Equal(t, submitJobRequest.EntryClass, "class")
		assert.Equal(t, submitJobRequest.SavepointPath, "location//")

		return &client.SubmitJobResponse{
			JobId: testJobId,
		}, nil
	}
	jobId, err := flinkControllerForTest.StartFlinkJob(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.Equal(t, jobId, testJobId)
}

func TestStartFlinkJobEmptyJobId(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.SubmitJobFunc = func(ctx context.Context, url string, jarId string, submitJobRequest client.SubmitJobRequest) (*client.SubmitJobResponse, error) {

		return &client.SubmitJobResponse{}, nil
	}
	jobId, err := flinkControllerForTest.StartFlinkJob(context.Background(), &flinkApp)
	assert.EqualError(t, err, "unable to submit job: invalid job id")
	assert.Empty(t, jobId)
}

func TestStartFlinkJobErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.SubmitJobFunc = func(ctx context.Context, url string, jarId string, submitJobRequest client.SubmitJobRequest) (*client.SubmitJobResponse, error) {
		return nil, errors.New("submit error")
	}
	jobId, err := flinkControllerForTest.StartFlinkJob(context.Background(), &flinkApp)
	assert.EqualError(t, err, "submit error")
	assert.Empty(t, jobId)
}

func TestCancelWithSavepoint(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.CancelJobWithSavepointFunc = func(ctx context.Context, url string, jobId string) (string, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		assert.Equal(t, jobId, testJobId)
		return "t1", nil
	}
	triggerId, err := flinkControllerForTest.CancelWithSavepoint(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.Equal(t, triggerId, "t1")
}

func TestCancelWithSavepointErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.CancelJobWithSavepointFunc = func(ctx context.Context, url string, jobId string) (string, error) {
		return "", errors.New("cancel error")
	}
	triggerId, err := flinkControllerForTest.CancelWithSavepoint(context.Background(), &flinkApp)
	assert.EqualError(t, err, "cancel error")
	assert.Empty(t, triggerId)
}

func TestGetJobsForApplication(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.GetJobsFunc = func(ctx context.Context, url string) (*client.GetJobsResponse, error) {
		assert.Equal(t, url, "http://app-name-jm.ns:8081")
		return &client.GetJobsResponse{
			Jobs: []client.FlinkJob{
				{
					JobId: testJobId,
				},
			},
		}, nil
	}
	jobs, err := flinkControllerForTest.GetJobsForApplication(context.Background(), &flinkApp)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(jobs))
	assert.Equal(t, jobs[0].JobId, testJobId)
}

func TestGetJobsForApplicationErr(t *testing.T) {
	flinkControllerForTest := getTestFlinkController()
	flinkApp := getFlinkTestApp()

	mockJmClient := flinkControllerForTest.flinkClient.(*clientMock.MockJobManagerClient)
	mockJmClient.GetJobsFunc = func(ctx context.Context, url string) (*client.GetJobsResponse, error) {
		return nil, errors.New("get jobs error")
	}
	jobs, err := flinkControllerForTest.GetJobsForApplication(context.Background(), &flinkApp)
	assert.EqualError(t, err, "get jobs error")
	assert.Nil(t, jobs)
}
