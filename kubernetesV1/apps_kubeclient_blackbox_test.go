package kubernetesV1_test

import (
	"encoding/json"
	"io/ioutil"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fabric8-services/fabric8-wit/app"
	"github.com/fabric8-services/fabric8-wit/kubernetesV1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	v1 "k8s.io/client-go/pkg/api/v1"
)

// Used for equality comparisons between float64s
const fltEpsilon = 0.00000001

// Path to JSON resources
const pathToTestJSON = "../test/kubernetes/"

type testKube struct {
	kubernetesV1.KubeRESTAPI // Allows us to only implement methods we'll use
	fixture                  *testFixture
	configMapHolder          *testConfigMap
	quotaHolder              *testResourceQuota
	rcHolder                 *testReplicationController
	podHolder                *testPod
}

type testFixture struct {
	cmInput    *configMapInput
	rqInput    *resourceQuotaInput
	rcInput    map[string]string     // namespace -> RC JSON file
	podInput   map[string]string     // namespace -> pod JSON file
	bcInput    string                // BC json file
	dcInput    deploymentConfigInput // app name -> namespace -> DC json file
	scaleInput deploymentConfigInput // app name -> namespace -> DC scale json file
	kube       *testKube
	os         *testOpenShift
}

type deploymentConfigInput map[string]map[string]string

var defaultDeploymentConfigInput = deploymentConfigInput{
	"myApp": {
		"my-run": "deploymentconfig-one.json",
	},
}

var defaultDeploymentScaleInput = deploymentConfigInput{
	"myApp": {
		"my-run": "deployment-scale.json",
	},
}

func (input deploymentConfigInput) getInput(appName string, envNS string) *string {
	inputForApp, pres := input[appName]
	if !pres {
		return nil
	}
	inputForEnv, pres := inputForApp[envNS]
	if !pres {
		return nil
	}
	return &inputForEnv
}

// Config Maps fakes

type configMapInput struct {
	data       map[string]string
	labels     map[string]string
	shouldFail bool
}

var defaultConfigMapInput *configMapInput = &configMapInput{
	labels: map[string]string{"provider": "fabric8"},
	data: map[string]string{
		"run":   "name: Run\nnamespace: my-run\norder: 1",
		"stage": "name: Stage\nnamespace: my-stage\norder: 0",
	},
}

type testConfigMap struct {
	corev1.ConfigMapInterface
	input     *configMapInput
	namespace string
	configMap *v1.ConfigMap
}

func (tk *testKube) ConfigMaps(ns string) corev1.ConfigMapInterface {
	input := tk.fixture.cmInput
	if input == nil {
		input = defaultConfigMapInput
	}
	result := &testConfigMap{
		input:     input,
		namespace: ns,
	}
	tk.configMapHolder = result
	return result
}

func (cm *testConfigMap) Get(name string, options metav1.GetOptions) (*v1.ConfigMap, error) {
	result := &v1.ConfigMap{
		Data: cm.input.data,
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: cm.input.labels,
		},
	}
	cm.configMap = result
	return result, nil
}

// Resource Quota fakes

type resourceQuotaInput struct {
	name       string
	namespace  string
	hard       map[v1.ResourceName]float64
	used       map[v1.ResourceName]float64
	shouldFail bool
}

var defaultResourceQuotaInput *resourceQuotaInput = &resourceQuotaInput{
	name:      "run",
	namespace: "my-run",
	hard: map[v1.ResourceName]float64{
		v1.ResourceLimitsCPU:    0.7,
		v1.ResourceLimitsMemory: 1024,
	},
	used: map[v1.ResourceName]float64{
		v1.ResourceLimitsCPU:    0.4,
		v1.ResourceLimitsMemory: 512,
	},
}

type testResourceQuota struct {
	corev1.ResourceQuotaInterface
	input     *resourceQuotaInput
	namespace string
	quota     *v1.ResourceQuota
}

func (tk *testKube) ResourceQuotas(ns string) corev1.ResourceQuotaInterface {
	input := tk.fixture.rqInput
	if input == nil {
		input = defaultResourceQuotaInput
	}
	result := &testResourceQuota{
		input:     input,
		namespace: ns,
	}
	tk.quotaHolder = result
	return result
}

func (rq *testResourceQuota) Get(name string, options metav1.GetOptions) (*v1.ResourceQuota, error) {
	if rq.input.hard == nil || rq.input.used == nil { // Used to indicate no quota object
		return nil, nil
	}
	hardQuantity, err := stringToQuantityMap(rq.input.hard)
	if err != nil {
		return nil, err
	}
	usedQuantity, err := stringToQuantityMap(rq.input.used)
	if err != nil {
		return nil, err
	}
	result := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: v1.ResourceQuotaStatus{
			Hard: hardQuantity,
			Used: usedQuantity,
		},
	}
	rq.quota = result
	return result, nil
}

func stringToQuantityMap(input map[v1.ResourceName]float64) (v1.ResourceList, error) {
	result := make(map[v1.ResourceName]resource.Quantity)
	for k, v := range input {
		strVal := strconv.FormatFloat(v, 'f', -1, 64)
		q, err := resource.ParseQuantity(strVal)
		if err != nil {
			return nil, err
		}
		result[k] = q
	}
	return result, nil
}

// Replication Controller fakes

var defaultReplicationControllerInput = map[string]string{
	"my-run": "replicationcontroller.json",
}

type testReplicationController struct {
	corev1.ReplicationControllerInterface
	inputFile string
	namespace string
}

func (tk *testKube) ReplicationControllers(ns string) corev1.ReplicationControllerInterface {
	input := tk.fixture.rcInput[ns]
	result := &testReplicationController{
		inputFile: input,
		namespace: ns,
	}
	tk.rcHolder = result
	return result
}

func (rc *testReplicationController) List(options metav1.ListOptions) (*v1.ReplicationControllerList, error) {
	var result v1.ReplicationControllerList
	if len(rc.inputFile) == 0 {
		// No matching RC
		return &result, nil
	}
	err := readJSON(rc.inputFile, &result)
	return &result, err
}

// Pod fakes

var defaultPodInput = map[string]string{
	"my-run": "pods.json",
}

type testPod struct {
	corev1.PodInterface
	inputFile  string
	namespace  string
	latestList *v1.PodList
}

func (tk *testKube) Pods(ns string) corev1.PodInterface {
	input := tk.fixture.podInput[ns]
	result := &testPod{
		inputFile: input,
		namespace: ns,
	}
	tk.podHolder = result
	return result
}

func (pod *testPod) List(options metav1.ListOptions) (*v1.PodList, error) {
	var result v1.PodList
	if len(pod.inputFile) == 0 {
		// No matching pods
		return &result, nil
	}
	err := readJSON(pod.inputFile, &result)
	pod.latestList = &result
	return pod.latestList, err
}

func (fixture *testFixture) GetKubeRESTAPI(config *kubernetesV1.KubeClientConfig) (kubernetesV1.KubeRESTAPI, error) {
	mock := &testKube{
		fixture: fixture,
	}
	fixture.kube = mock
	return mock, nil
}

type testMetricsGetter struct {
	config *kubernetesV1.MetricsClientConfig
	input  *metricsInput
	result *testMetrics
}

type metricsHolder struct {
	pods      []v1.Pod
	namespace string
	startTime time.Time
	endTime   time.Time
	limit     int
}

type metricsInput struct {
	cpu    []*app.TimedNumberTupleV1
	memory []*app.TimedNumberTupleV1
	netTx  []*app.TimedNumberTupleV1
	netRx  []*app.TimedNumberTupleV1
}

var defaultMetricsInput = &metricsInput{
	cpu: []*app.TimedNumberTupleV1{
		createTuple(1.2, 1517867612000),
		createTuple(0.7, 1517867613000),
	},
	memory: []*app.TimedNumberTupleV1{
		createTuple(1200, 1517867612000),
		createTuple(3000, 1517867613000),
	},
	netTx: []*app.TimedNumberTupleV1{
		createTuple(300, 1517867612000),
		createTuple(520, 1517867613000),
	},
	netRx: []*app.TimedNumberTupleV1{
		createTuple(700, 1517867612000),
		createTuple(100, 1517867613000),
	},
}

func createTuple(val float64, ts float64) *app.TimedNumberTupleV1 {
	return &app.TimedNumberTupleV1{
		Value: &val,
		Time:  &ts,
	}
}

type testMetrics struct {
	fixture     *testMetricsGetter
	cpuParams   *metricsHolder
	memParams   *metricsHolder
	netTxParams *metricsHolder
	netRxParams *metricsHolder
	closed      bool
}

func (fixture *testMetricsGetter) GetMetrics(config *kubernetesV1.MetricsClientConfig) (kubernetesV1.MetricsInterface, error) {
	metrics := &testMetrics{
		fixture: fixture,
	}
	fixture.config = config
	fixture.result = metrics
	return metrics, nil
}

func (tm *testMetrics) GetCPUMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return tm.getOneMetric(tm.fixture.input.cpu, pods, namespace, startTime, &tm.cpuParams)
}

func (tm *testMetrics) GetCPUMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return tm.getManyMetrics(tm.fixture.input.cpu, pods, namespace, startTime, endTime, limit, &tm.cpuParams)
}

func (tm *testMetrics) GetMemoryMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return tm.getOneMetric(tm.fixture.input.memory, pods, namespace, startTime, &tm.memParams)
}

func (tm *testMetrics) GetMemoryMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return tm.getManyMetrics(tm.fixture.input.memory, pods, namespace, startTime, endTime, limit, &tm.memParams)
}

func (tm *testMetrics) GetNetworkSentMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return tm.getOneMetric(tm.fixture.input.netTx, pods, namespace, startTime, &tm.netTxParams)
}

func (tm *testMetrics) GetNetworkSentMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return tm.getManyMetrics(tm.fixture.input.netTx, pods, namespace, startTime, endTime, limit, &tm.netTxParams)
}

func (tm *testMetrics) GetNetworkRecvMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return tm.getOneMetric(tm.fixture.input.netRx, pods, namespace, startTime, &tm.netRxParams)
}

func (tm *testMetrics) GetNetworkRecvMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return tm.getManyMetrics(tm.fixture.input.netRx, pods, namespace, startTime, endTime, limit, &tm.netRxParams)
}

func (tm *testMetrics) getOneMetric(metrics []*app.TimedNumberTupleV1, pods []v1.Pod, namespace string,
	startTime time.Time, holderPtr **metricsHolder) (*app.TimedNumberTupleV1, error) {
	*holderPtr = &metricsHolder{
		pods:      pods,
		namespace: namespace,
		startTime: startTime,
	}
	return metrics[0], nil
}

func (tm *testMetrics) Close() {
	tm.closed = true
}

func (tm *testMetrics) getManyMetrics(metrics []*app.TimedNumberTupleV1, pods []v1.Pod, namespace string,
	startTime time.Time, endTime time.Time, limit int, holderPtr **metricsHolder) ([]*app.TimedNumberTupleV1, error) {
	*holderPtr = &metricsHolder{
		pods:      pods,
		namespace: namespace,
		startTime: startTime,
		endTime:   endTime,
		limit:     limit,
	}
	return metrics, nil
}
func TestGetMetrics(t *testing.T) {
	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}

	token := "myToken"
	testCases := []struct {
		clusterURL    string
		expectedURL   string
		shouldSucceed bool
	}{
		{"https://api.myCluster.url:443/cluster", "https://metrics.myCluster.url", true},
		{"https://myCluster.url:443/cluster", "", false},
	}

	for _, testCase := range testCases {
		config := &kubernetesV1.KubeClientConfig{
			ClusterURL:        testCase.clusterURL,
			BearerToken:       token,
			UserNamespace:     "myNamespace",
			KubeRESTAPIGetter: fixture,
			MetricsGetter:     metricsGetter,
		}
		_, err := kubernetesV1.NewKubeClient(config)
		if testCase.shouldSucceed {
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, testCase.expectedURL, metricsGetter.config.MetricsURL, "Incorrect Metrics URL")
			assert.Equal(t, token, metricsGetter.config.BearerToken, "Incorrect bearer token")
		} else {
			if err == nil {
				t.Error("Expected error, but was successful")
			}
		}
	}
}

func TestClose(t *testing.T) {
	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}

	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     "myNamespace",
		KubeRESTAPIGetter: fixture,
		MetricsGetter:     metricsGetter,
	}
	client, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err, "Failed to create Kubernetes client")

	// Check that KubeClientInterface.Close invokes MetricsInterface.Close
	client.Close()
	assert.True(t, metricsGetter.result.closed, "Metrics client not closed")
}

func TestConfigMapEnvironments(t *testing.T) {
	testCases := []*configMapInput{
		{
			labels: map[string]string{"provider": "fabric8"},
			data: map[string]string{
				"run":   "name: Run\nnamespace: my-run\norder: 1",
				"stage": "name: Stage\nnamespace: my-stage\norder: 0",
			},
		},
		{
			labels: map[string]string{"provider": "fabric8"},
			data:   map[string]string{},
		},
		{
			labels: map[string]string{"provider": "fabric8"},
			data: map[string]string{
				"run": "name: Run\nnamespace my-run\norder: 1", // Missing colon
			},
			shouldFail: true,
		},
		{
			labels: map[string]string{"provider": "fabric8"},
			data: map[string]string{
				"run": "name: Run\nns: my-run\norder: 1", // Missing namespace
			},
			shouldFail: true,
		},
		{
			shouldFail: true, // No provider
		},
	}
	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	userNamespace := "myNamespace"
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     userNamespace,
		KubeRESTAPIGetter: fixture,
		MetricsGetter:     metricsGetter,
	}

	expectedName := "fabric8-environments"
	for _, testCase := range testCases {
		fixture.cmInput = testCase
		_, err := kubernetesV1.NewKubeClient(config)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			configMapHolder := fixture.kube.configMapHolder
			if !assert.NotNil(t, configMapHolder, "No ConfigMap created by test") {
				continue
			}
			assert.Equal(t, userNamespace, configMapHolder.namespace, "ConfigMap obtained from wrong namespace")
			configMap := configMapHolder.configMap
			if !assert.NotNil(t, configMap, "Never sent ConfigMap GET") {
				continue
			}
			assert.Equal(t, expectedName, configMap.Name, "Incorrect ConfigMap name")
		}
	}
}

func TestGetEnvironment(t *testing.T) {
	testCases := []*resourceQuotaInput{
		{
			name:      "run",
			namespace: "my-run",
			hard: map[v1.ResourceName]float64{
				v1.ResourceLimitsCPU:    0.7,
				v1.ResourceLimitsMemory: 1024,
			},
			used: map[v1.ResourceName]float64{
				v1.ResourceLimitsCPU:    0.4,
				v1.ResourceLimitsMemory: 512,
			},
		},
		{
			name:      "doesNotExist", // Bad environment name
			namespace: "my-run",
			hard: map[v1.ResourceName]float64{
				v1.ResourceLimitsCPU:    0.7,
				v1.ResourceLimitsMemory: 1024,
			},
			used: map[v1.ResourceName]float64{
				v1.ResourceLimitsCPU:    0.4,
				v1.ResourceLimitsMemory: 512,
			},
			shouldFail: true,
		},
		{
			name:       "run",
			namespace:  "my-run",
			shouldFail: true, // No quantities, so our test impl returns nil
		},
	}
	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     "myNamespace",
		KubeRESTAPIGetter: fixture,
		MetricsGetter:     metricsGetter,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	assert.NoError(t, err)
	for _, testCase := range testCases {
		fixture.rqInput = testCase
		env, err := kc.GetEnvironment(testCase.name)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}

			quotaHolder := fixture.kube.quotaHolder
			if !assert.NotNil(t, quotaHolder, "No ResourceQuota created by test") {
				continue
			}
			assert.Equal(t, testCase.namespace, quotaHolder.namespace, "Quota retrieved from wrong namespace")
			quota := quotaHolder.quota
			if !assert.NotNil(t, quota, "Never sent ResourceQuota GET") {
				continue
			}
			assert.Equal(t, "compute-resources", quota.Name, "Wrong ResourceQuota name")
			assert.Equal(t, testCase.name, *env.Name, "Wrong environment name")

			cpuQuota := env.Quota.Cpucores
			assert.InEpsilon(t, testCase.hard[v1.ResourceLimitsCPU], *cpuQuota.Quota, fltEpsilon, "Incorrect CPU quota")
			assert.InEpsilon(t, testCase.used[v1.ResourceLimitsCPU], *cpuQuota.Used, fltEpsilon, "Incorrect CPU usage")

			memQuota := env.Quota.Memory
			assert.InEpsilon(t, testCase.hard[v1.ResourceLimitsMemory], *memQuota.Quota, fltEpsilon, "Incorrect memory quota")
			assert.InEpsilon(t, testCase.used[v1.ResourceLimitsMemory], *memQuota.Used, fltEpsilon, "Incorrect memory usage")
		}
	}
}

type testOpenShift struct {
	fixture     *testFixture
	scaleHolder *testScale
}

type testScale struct {
	scaleOutput map[string]interface{}
	namespace   string
	dcName      string
}

type spaceTestData struct {
	name       string
	shouldFail bool
	bcJson     string
	appInput   map[string]*appTestData // Keys are app names
	dcInput    deploymentConfigInput
	rcInput    map[string]string
	podInput   map[string]string
}

const defaultBuildConfig = "buildconfigs-one.json"

var defaultSpaceTestData = &spaceTestData{
	name:     "mySpace",
	bcJson:   defaultBuildConfig,
	appInput: map[string]*appTestData{"myApp": defaultAppTestData},
	dcInput:  defaultDeploymentConfigInput,
	rcInput:  defaultReplicationControllerInput,
	podInput: defaultPodInput,
}

type appTestData struct {
	spaceName   string
	appName     string
	shouldFail  bool
	deployInput map[string]*deployTestData // Keys are environment names
	dcInput     deploymentConfigInput
	rcInput     map[string]string
	podInput    map[string]string
}

var defaultAppTestData = &appTestData{
	spaceName:   "mySpace",
	appName:     "myApp",
	deployInput: map[string]*deployTestData{"run": defaultDeployTestData},
	dcInput:     defaultDeploymentConfigInput,
	rcInput:     defaultReplicationControllerInput,
	podInput:    defaultPodInput,
}

type deployTestData struct {
	spaceName          string
	appName            string
	envName            string
	envNS              string
	expectVersion      string
	expectPodsRunning  int
	expectPodsStarting int
	expectPodsStopping int
	expectPodsTotal    int
	shouldFail         bool
	dcInput            deploymentConfigInput
	rcInput            map[string]string
	podInput           map[string]string
}

var defaultDeployTestData = &deployTestData{
	spaceName:         "mySpace",
	appName:           "myApp",
	envName:           "run",
	envNS:             "my-run",
	expectVersion:     "1.0.2",
	expectPodsRunning: 2,
	expectPodsTotal:   2,
	dcInput:           defaultDeploymentConfigInput,
	rcInput:           defaultReplicationControllerInput,
	podInput:          defaultPodInput,
}

type deployStatsTestData struct {
	spaceName    string
	appName      string
	envName      string
	envNS        string
	shouldFail   bool
	metricsInput *metricsInput
	startTime    time.Time
	endTime      time.Time
	expectStart  int64
	expectEnd    int64
	limit        int
	dcInput      deploymentConfigInput
	rcInput      map[string]string
	podInput     map[string]string
}

var defaultDeployStatsTestData = &deployStatsTestData{
	spaceName:    "mySpace",
	appName:      "myApp",
	envName:      "run",
	envNS:        "my-run",
	startTime:    convertToTime(1517867603000),
	endTime:      convertToTime(1517867643000),
	expectStart:  1517867612000,
	expectEnd:    1517867613000,
	limit:        10,
	metricsInput: defaultMetricsInput,
	dcInput:      defaultDeploymentConfigInput,
	rcInput:      defaultReplicationControllerInput,
	podInput:     defaultPodInput,
}

func convertToTime(unixMillis int64) time.Time {
	return time.Unix(0, unixMillis*int64(time.Millisecond))
}

func (fixture *testFixture) GetOpenShiftRESTAPI(config *kubernetesV1.KubeClientConfig) (kubernetesV1.OpenShiftRESTAPI, error) {
	oapi := &testOpenShift{
		fixture: fixture,
	}
	fixture.os = oapi
	return oapi, nil
}

func (to *testOpenShift) GetBuildConfigs(namespace string, labelSelector string) (map[string]interface{}, error) {
	var result map[string]interface{}
	input := to.fixture.bcInput
	if len(input) == 0 {
		// No matching BCs
		return result, nil
	}
	err := readJSON(input, &result)
	return result, err
}

func (to *testOpenShift) GetDeploymentConfig(namespace string, name string) (map[string]interface{}, error) {
	input := to.fixture.dcInput.getInput(name, namespace)
	if input == nil {
		// No matching DC
		return nil, nil
	}
	var result map[string]interface{}
	err := readJSON(*input, &result)
	return result, err
}

func (to *testOpenShift) GetDeploymentConfigScale(namespace string, name string) (map[string]interface{}, error) {
	input := to.fixture.scaleInput.getInput(name, namespace)
	if input == nil {
		// No matching DC scale
		return nil, nil
	}
	var result map[string]interface{}
	err := readJSON(*input, &result)
	return result, err
}

func (to *testOpenShift) SetDeploymentConfigScale(namespace string, name string, scale map[string]interface{}) error {
	to.scaleHolder = &testScale{
		namespace:   namespace,
		dcName:      name,
		scaleOutput: scale,
	}
	return nil
}

func readJSON(filename string, dest interface{}) error {
	path := pathToTestJSON + filename
	jsonBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonBytes, dest)
	if err != nil {
		return err
	}
	return nil
}

func TestGetSpace(t *testing.T) {
	testCases := []*spaceTestData{
		{
			name:   "mySpace",
			bcJson: "buildconfigs-emptylist.json",
		},
		{
			name:       "mySpace",
			bcJson:     "buildconfigs-wronglist.json",
			shouldFail: true,
		},
		{
			name:       "mySpace",
			bcJson:     "buildconfigs-noitems.json",
			shouldFail: true,
		},
		{
			name:       "mySpace",
			bcJson:     "buildconfigs-notobject.json",
			shouldFail: true,
		},
		{
			name:       "mySpace",
			bcJson:     "buildconfigs-nometadata.json",
			shouldFail: true,
		},
		{
			name:       "mySpace",
			bcJson:     "buildconfigs-noname.json",
			shouldFail: true,
		},
		defaultSpaceTestData,
		{
			name:   "mySpace",
			bcJson: "buildconfigs-two.json",
			appInput: map[string]*appTestData{
				"myApp": defaultAppTestData,
				"myOtherApp": &appTestData{
					spaceName: "mySpace",
					appName:   "myOtherApp",
				},
			},
			dcInput:  defaultDeploymentConfigInput,
			rcInput:  defaultReplicationControllerInput,
			podInput: defaultPodInput,
		}, // TODO test >1 apps with >1 deployments
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.bcInput = testCase.bcJson
		fixture.dcInput = testCase.dcInput
		fixture.rcInput = testCase.rcInput
		fixture.podInput = testCase.podInput

		space, err := kc.GetSpace(testCase.name)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			assert.NotNil(t, space, "Space is nil")
			assert.Equal(t, testCase.name, *space.Name, "Space name is incorrect")
			assert.NotNil(t, space.Applications, "Applications are nil")
			for _, app := range space.Applications {
				var appInput *appTestData
				if app != nil && app.Name != nil {
					appInput = testCase.appInput[*app.Name]
					require.NotNil(t, appInput, "Unknown app: "+*app.Name)
				}
				verifyApplication(app, appInput, t)
			}
		}
	}
}

func TestGetApplication(t *testing.T) {
	dcInput := deploymentConfigInput{
		"myApp": {
			"my-run":   "deploymentconfig-one.json",
			"my-stage": "deploymentconfig-one-stage.json",
		},
	}
	rcInput := map[string]string{
		"my-run":   "replicationcontroller.json",
		"my-stage": "replicationcontroller.json",
	}
	podInput := map[string]string{
		"my-run":   "pods.json",
		"my-stage": "pods-one-stopped.json",
	}
	testCases := []*appTestData{
		defaultAppTestData, // TODO test multiple deployments
		{
			spaceName: "mySpace",
			appName:   "myApp",
			deployInput: map[string]*deployTestData{
				"run": defaultDeployTestData,
				"stage": {
					spaceName:          "mySpace",
					appName:            "myApp",
					envName:            "stage",
					envNS:              "my-stage",
					expectVersion:      "1.0.3",
					expectPodsRunning:  1,
					expectPodsStopping: 1,
					expectPodsTotal:    2,
				},
			},
			dcInput:  dcInput,
			rcInput:  rcInput,
			podInput: podInput,
		},
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.dcInput = testCase.dcInput
		fixture.rcInput = testCase.rcInput
		fixture.podInput = testCase.podInput

		app, err := kc.GetApplication(testCase.spaceName, testCase.appName)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			verifyApplication(app, testCase, t)
		}
	}
}

func TestGetDeployment(t *testing.T) {
	testCases := []*deployTestData{
		defaultDeployTestData,
		{
			spaceName:  "mySpace",
			appName:    "myApp",
			envName:    "doesNotExist",
			dcInput:    defaultDeploymentConfigInput,
			rcInput:    defaultReplicationControllerInput,
			podInput:   defaultPodInput,
			shouldFail: true,
		},
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.dcInput = testCase.dcInput
		fixture.rcInput = testCase.rcInput
		fixture.podInput = testCase.podInput

		dep, err := kc.GetDeployment(testCase.spaceName, testCase.appName, testCase.envName)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			verifyDeployment(dep, testCase, t)
		}
	}
}

func TestScaleDeployment(t *testing.T) {
	testCases := []struct {
		spaceName   string
		appName     string
		envName     string
		expectedNS  string
		dcInput     deploymentConfigInput
		scaleInput  deploymentConfigInput
		newReplicas int
		oldReplicas int
		shouldFail  bool
	}{
		{
			spaceName:   "mySpace",
			appName:     "myApp",
			envName:     "run",
			expectedNS:  "my-run",
			dcInput:     defaultDeploymentConfigInput,
			scaleInput:  defaultDeploymentScaleInput,
			newReplicas: 3,
			oldReplicas: 2,
		},
		{
			spaceName:  "mySpace",
			appName:    "myApp",
			envName:    "run",
			expectedNS: "my-run",
			dcInput:    defaultDeploymentConfigInput,
			scaleInput: deploymentConfigInput{
				"myApp": {
					"my-run": "deployment-scale-zero.json",
				},
			},
			newReplicas: 1,
			oldReplicas: 0,
		},
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.dcInput = testCase.dcInput
		fixture.scaleInput = testCase.scaleInput

		old, err := kc.ScaleDeployment(testCase.spaceName, testCase.appName, testCase.envName, testCase.newReplicas)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			assert.NotNil(t, old, "Previous replicas are nil")
			assert.Equal(t, testCase.oldReplicas, *old, "Wrong number of previous replicas")
			scaleHolder := fixture.os.scaleHolder
			assert.NotNil(t, scaleHolder, "No scale results available")
			assert.Equal(t, testCase.expectedNS, scaleHolder.namespace, "Wrong namespace")
			assert.Equal(t, testCase.appName, scaleHolder.dcName, "Wrong deployment config name")
			// Check spec/replicas modified correctly
			spec, ok := scaleHolder.scaleOutput["spec"].(map[string]interface{})
			assert.True(t, ok, "Spec property is missing or invalid")
			newReplicas, ok := spec["replicas"].(int)
			assert.True(t, ok, "Replicas property is missing or invalid")
			assert.Equal(t, testCase.newReplicas, newReplicas, "Wrong modified number of replicas")
		}
	}
}

func TestGetDeploymentStats(t *testing.T) {
	testCases := []*deployStatsTestData{
		defaultDeployStatsTestData,
		{
			spaceName:    "mySpace",
			appName:      "myApp",
			envName:      "doesNotExist",
			metricsInput: defaultMetricsInput,
			dcInput:      defaultDeploymentConfigInput,
			rcInput:      defaultReplicationControllerInput,
			podInput:     defaultPodInput,
			shouldFail:   true,
		},
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.dcInput = testCase.dcInput
		fixture.rcInput = testCase.rcInput
		fixture.podInput = testCase.podInput
		metricsGetter.input = testCase.metricsInput

		stats, err := kc.GetDeploymentStats(testCase.spaceName, testCase.appName, testCase.envName, testCase.startTime)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}

			assert.NotNil(t, stats, "GetDeploymentStats returned nil")
			result := metricsGetter.result
			assert.NotNil(t, result, "Metrics API not called")
			// Check each method called with pods returned by Kube API
			assert.NotNil(t, fixture.kube, "Kube results are nil")
			assert.NotNil(t, fixture.kube.podHolder, "Pods API not called")
			assert.NotNil(t, fixture.kube.podHolder.latestList, "No pod list retrieved")
			pods := fixture.kube.podHolder.latestList.Items

			// Check each metric type
			assert.Equal(t, testCase.metricsInput.cpu[0], stats.Cores, "Incorrect CPU metrics returned")
			verifyMetricsParams(testCase, result.cpuParams, pods, t, "CPU metrics")
			assert.Equal(t, testCase.metricsInput.memory[0], stats.Memory, "Incorrect memory metrics returned")
			verifyMetricsParams(testCase, result.memParams, pods, t, "Memory metrics")
			assert.Equal(t, testCase.metricsInput.netTx[0], stats.NetTx, "Incorrect network sent metrics returned")
			verifyMetricsParams(testCase, result.netTxParams, pods, t, "Network sent metrics")
			assert.Equal(t, testCase.metricsInput.netRx[0], stats.NetRx, "Incorrect network received metrics returned")
			verifyMetricsParams(testCase, result.netRxParams, pods, t, "Network received metrics")
		}
	}
}

func TestGetDeploymentStatSeries(t *testing.T) {
	testCases := []*deployStatsTestData{
		defaultDeployStatsTestData,
		{
			spaceName:    "mySpace",
			appName:      "myApp",
			envName:      "doesNotExist",
			metricsInput: defaultMetricsInput,
			dcInput:      defaultDeploymentConfigInput,
			rcInput:      defaultReplicationControllerInput,
			podInput:     defaultPodInput,
			shouldFail:   true,
		},
	}

	fixture := &testFixture{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      fixture,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: fixture,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		fixture.dcInput = testCase.dcInput
		fixture.rcInput = testCase.rcInput
		fixture.podInput = testCase.podInput
		metricsGetter.input = testCase.metricsInput

		stats, err := kc.GetDeploymentStatSeries(testCase.spaceName, testCase.appName, testCase.envName,
			testCase.startTime, testCase.endTime, testCase.limit)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}

			assert.NotNil(t, stats, "GetDeploymentStats returned nil")
			result := metricsGetter.result
			assert.NotNil(t, result, "Metrics API not called")
			// Check each method called with pods returned by Kube API
			assert.NotNil(t, fixture.kube, "Kube results are nil")
			assert.NotNil(t, fixture.kube.podHolder, "Pods API not called")
			assert.NotNil(t, fixture.kube.podHolder.latestList, "No pod list retrieved")
			pods := fixture.kube.podHolder.latestList.Items

			// Check each metric type
			assert.Equal(t, testCase.metricsInput.cpu, stats.Cores, "Incorrect CPU metrics returned")
			verifyMetricsParams(testCase, result.cpuParams, pods, t, "CPU metrics")
			assert.Equal(t, testCase.metricsInput.memory, stats.Memory, "Incorrect memory metrics returned")
			verifyMetricsParams(testCase, result.memParams, pods, t, "Memory metrics")
			assert.Equal(t, testCase.metricsInput.netTx, stats.NetTx, "Incorrect network sent metrics returned")
			verifyMetricsParams(testCase, result.netTxParams, pods, t, "Network sent metrics")
			assert.Equal(t, testCase.metricsInput.netRx, stats.NetRx, "Incorrect network received metrics returned")
			verifyMetricRangeParams(testCase, result.netRxParams, pods, t, "Network received metrics")

			// Check time range
			assert.Equal(t, testCase.expectStart, int64(*stats.Start), "Incorrect start time")
			assert.Equal(t, testCase.expectEnd, int64(*stats.End), "Incorrect end time")
		}
	}
}

func verifyMetricsParams(testCase *deployStatsTestData, params *metricsHolder, expectPods []v1.Pod, t *testing.T,
	metricName string) {
	assert.Equal(t, testCase.envNS, params.namespace, metricName+" called with wrong namespace")
	assert.Equal(t, testCase.startTime, params.startTime, metricName+" called with wrong start time")
	assert.Equal(t, expectPods, params.pods, metricName+" called with unexpected pods")
}

func verifyMetricRangeParams(testCase *deployStatsTestData, params *metricsHolder, expectPods []v1.Pod, t *testing.T,
	metricName string) {
	verifyMetricsParams(testCase, params, expectPods, t, metricName)
	assert.Equal(t, testCase.endTime, params.endTime, metricName+" called with wrong end time")
	assert.Equal(t, testCase.limit, params.limit, metricName+" called with wrong limit")
}

func verifyApplication(app *app.SimpleAppV1, testCase *appTestData, t *testing.T) {
	if !assert.NotNil(t, app, "Application is nil") {
		return
	}
	assert.NotNil(t, app.Name, "Application name is nil")
	assert.Equal(t, testCase.appName, *app.Name, "Incorrect application name")
	assert.NotNil(t, app.Pipeline, "Deployments are nil")
	if !assert.Equal(t, len(testCase.deployInput), len(app.Pipeline), "Wrong number of deployments") {
		return
	}
	for _, dep := range app.Pipeline {
		var depInput *deployTestData
		if dep != nil && dep.Name != nil {
			depInput = testCase.deployInput[*dep.Name]
			require.NotNil(t, depInput, "Unknown env: "+*dep.Name)
		}
		verifyDeployment(dep, depInput, t)
	}
}

func verifyDeployment(dep *app.SimpleDeploymentV1, testCase *deployTestData, t *testing.T) {
	if !assert.NotNil(t, dep, "Deployment is nil") {
		return
	}
	if assert.NotNil(t, dep.Name, "Deployment name is nil") {
		assert.Equal(t, testCase.envName, *dep.Name, "Incorrect deployment name")
	}
	if assert.NotNil(t, dep.Version, "Deployments version is nil") {
		assert.Equal(t, testCase.expectVersion, *dep.Version, "Incorrect deployment version")
	}
	// TODO use assert.ElementsMatch when moving to new pod status format
	if assert.NotNil(t, dep.Pods, "Pods are nil") {
		assert.Equal(t, testCase.expectPodsRunning, *dep.Pods.Running, "Wrong number of running pods")
		assert.Equal(t, testCase.expectPodsStarting, *dep.Pods.Starting, "Wrong number of starting pods")
		assert.Equal(t, testCase.expectPodsStopping, *dep.Pods.Stopping, "Wrong number of stopping pods")
		assert.Equal(t, testCase.expectPodsTotal, *dep.Pods.Total, "Wrong number of total pods")
	}
}
