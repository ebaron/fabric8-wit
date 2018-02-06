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

type testKube struct {
	kubernetesV1.KubeRESTAPI // Allows us to only implement methods we'll use
	getter                   *testKubeGetter
	configMapHolder          *testConfigMap
	quotaHolder              *testResourceQuota
}

type testKubeGetter struct {
	cmInput *configMapInput
	rqInput *resourceQuotaInput
	result  *testKube
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
	input := tk.getter.cmInput
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
	input := tk.getter.rqInput
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

func (getter *testKubeGetter) GetKubeRESTAPI(config *kubernetesV1.KubeClientConfig) (kubernetesV1.KubeRESTAPI, error) {
	mock := new(testKube)
	// Doubly-linked for access by tests
	mock.getter = getter
	getter.result = mock
	return mock, nil
}

type testMetricsGetter struct {
	config *kubernetesV1.MetricsClientConfig
	result *testMetrics
}

type testMetrics struct {
	closed bool
}

func (getter *testMetricsGetter) GetMetrics(config *kubernetesV1.MetricsClientConfig) (kubernetesV1.MetricsInterface, error) {
	getter.config = config
	getter.result = &testMetrics{}
	return getter.result, nil
}

func (tm *testMetrics) Close() {
	tm.closed = true
}

func (tm *testMetrics) GetCPUMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO
}

func (tm *testMetrics) GetCPUMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO
}

func (tm *testMetrics) GetMemoryMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO
}

func (tm *testMetrics) GetMemoryMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO
}

func (testMetrics) GetNetworkSentMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO add fake impl when tests exercise this code
}

func (testMetrics) GetNetworkSentMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO add fake impl when tests exercise this code
}

func (testMetrics) GetNetworkRecvMetrics(pods []v1.Pod, namespace string, startTime time.Time) (*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO add fake impl when tests exercise this code
}

func (testMetrics) GetNetworkRecvMetricsRange(pods []v1.Pod, namespace string, startTime time.Time, endTime time.Time,
	limit int) ([]*app.TimedNumberTupleV1, error) {
	return nil, nil // TODO add fake impl when tests exercise this code
}

func TestGetMetrics(t *testing.T) {
	kubeGetter := &testKubeGetter{
		cmInput: defaultConfigMapInput,
	}
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
			KubeRESTAPIGetter: kubeGetter,
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
	kubeGetter := &testKubeGetter{}
	metricsGetter := &testMetricsGetter{}

	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     "myNamespace",
		KubeRESTAPIGetter: kubeGetter,
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
	kubeGetter := &testKubeGetter{}
	metricsGetter := &testMetricsGetter{}
	userNamespace := "myNamespace"
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     userNamespace,
		KubeRESTAPIGetter: kubeGetter,
		MetricsGetter:     metricsGetter,
	}

	expectedName := "fabric8-environments"
	for _, testCase := range testCases {
		kubeGetter.cmInput = testCase
		_, err := kubernetesV1.NewKubeClient(config)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			configMapHolder := kubeGetter.result.configMapHolder
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
	kubeGetter := &testKubeGetter{}
	metricsGetter := &testMetricsGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:        "http://api.myCluster",
		BearerToken:       "myToken",
		UserNamespace:     "myNamespace",
		KubeRESTAPIGetter: kubeGetter,
		MetricsGetter:     metricsGetter,
	}

	kc, err := kubernetesV1.NewKubeClient(config)
	assert.NoError(t, err)
	for _, testCase := range testCases {
		kubeGetter.rqInput = testCase
		env, err := kc.GetEnvironment(testCase.name)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}

			quotaHolder := kubeGetter.result.quotaHolder
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
	getter *testOpenShiftGetter
}

type testOpenShiftGetter struct {
	result  *testOpenShift
	spInput *spaceTestData
}

type spaceTestData struct {
	name       string
	shouldFail bool
	bcJson     string
}

const pathToTestJSON = "../test/kubernetes/"

func (getter *testOpenShiftGetter) GetOpenShiftRESTAPI(config *kubernetes.KubeClientConfig) (kubernetes.OpenShiftRESTAPI, error) {
	oapi := &testOpenShift{
		getter: getter,
	}
	return oapi, nil
}

func (to *testOpenShift) GetBuildConfigs(namespace string, labelSelector string) (map[string]interface{}, error) {
	filename := pathToTestJSON + to.getter.spInput.bcJson
	return readJSON(filename)
}

func (to *testOpenShift) GetDeploymentConfig(namespace string, name string) (map[string]interface{}, error) {
	return nil, nil // TODO
}

func (to *testOpenShift) GetDeploymentConfigScale(namespace string, name string) (map[string]interface{}, error) {
	return nil, nil // TODO
}

func (to *testOpenShift) SetDeploymentConfigScale(namespace string, name string, scale map[string]interface{}) error {
	return nil // TODO
}

func readJSON(filename string) (map[string]interface{}, error) {
	jsonBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func TestGetSpaceWithNoConfigs(t *testing.T) {
	testCases := []*spaceTestData{
		{
			name:   "run",
			bcJson: "buildconfigs-emptylist.json",
		},
		{
			name:       "run",
			bcJson:     "buildconfigs-wronglist.json",
			shouldFail: true,
		},
		{
			name:       "run",
			bcJson:     "buildconfigs-noitems.json",
			shouldFail: true,
		},
		{
			name:       "run",
			bcJson:     "buildconfigs-notobject.json",
			shouldFail: true,
		},
		{
			name:       "run",
			bcJson:     "buildconfigs-nometadata.json",
			shouldFail: true,
		},
		{
			name:       "run",
			bcJson:     "buildconfigs-noname.json",
			shouldFail: true,
		},
		{
			name:   "run",
			bcJson: "buildconfigs-two.json",
		},
	}

	kubeGetter := &testKubeGetter{}
	metricsGetter := &testMetricsGetter{}
	openShiftGetter := &testOpenShiftGetter{}
	config := &kubernetesV1.KubeClientConfig{
		ClusterURL:             "http://api.myCluster",
		BearerToken:            "myToken",
		UserNamespace:          "myNamespace",
		KubeRESTAPIGetter:      kubeGetter,
		MetricsGetter:          metricsGetter,
		OpenShiftRESTAPIGetter: openShiftGetter,
	}

	kc, err := kubernetes.NewKubeClient(config)
	require.NoError(t, err)

	for _, testCase := range testCases {
		openShiftGetter.spInput = testCase
		space, err := kc.GetSpace(testCase.name)
		if testCase.shouldFail {
			assert.Error(t, err)
		} else {
			if !assert.NoError(t, err) {
				continue
			}
			assert.NotNil(t, space.Applications)
			// TODO test applications
		}
	}
}
