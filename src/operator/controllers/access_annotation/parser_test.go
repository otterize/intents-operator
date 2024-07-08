package access_annotation

import (
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

type PodAnnotationParserSuite struct {
	suite.Suite
}

func (s *PodAnnotationParserSuite) TestGetIntentsFromAccessAnnotation() {
	annotationString := `[{
		"name": "prometheus.infra",
		"kind": "Deployment"
	},
	{
		"kind": "StatefulSet",
		"name": "haproxy.infra"
	},
	{
		"name": "another-server.another-namespace",
		"kind": "MyCustomKind"
	}]`

	serviceIdentities, err := parseAnnotation(annotationString)
	s.Require().NoError(err)
	s.Require().Equal(3, len(serviceIdentities))
	s.Require().Equal("prometheus", serviceIdentities[0].Name)
	s.Require().Equal("infra", serviceIdentities[0].Namespace)
	s.Require().Equal("Deployment", serviceIdentities[0].Kind)

	s.Require().Equal("haproxy", serviceIdentities[1].Name)
	s.Require().Equal("infra", serviceIdentities[1].Namespace)
	s.Require().Equal("StatefulSet", serviceIdentities[1].Kind)

	s.Require().Equal("another-server", serviceIdentities[2].Name)
	s.Require().Equal("another-namespace", serviceIdentities[2].Namespace)
	s.Require().Equal("MyCustomKind", serviceIdentities[2].Kind)
}

func (s *PodAnnotationParserSuite) TestParseError() {
	type testCase struct {
		annotation string
		name       string
	}

	testCases := []testCase{
		{
			name:       "emptyObject",
			annotation: `[{}]`,
		},
		{
			name:       "invalidJSON",
			annotation: `[{]`,
		},
		{
			name:       "lowercase kind",
			annotation: `[{"name": "prometheus.infra","kind": "deployment"}]`,
		},
		{
			name:       "missingName",
			annotation: `[{"kind": "Deployment"}]`,
		},
		{
			name:       "missingKind",
			annotation: `[{"name": "prometheus.infra"}]`,
		},
		{
			name:       "noNamespace",
			annotation: `[{"name": "prometheusinfra","kind": "Deployment"}]`,
		},
		{
			name:       "multipleDotes",
			annotation: `[{"name": "prometheus.infra.test","kind": "Deployment"}]`,
		},
		{
			name:       "emptyObject",
			annotation: `[{}]`,
		},
	}

	for _, tc := range testCases {
		_, err := parseAnnotation(tc.annotation)
		s.Require().Error(err, "parsing test case failed: %s", tc.name)
	}
}

func (s *PodAnnotationParserSuite) TestGetAccessAnnotationFromPod() {
	podName := "pod-1"
	testNamespace := "test-namespace"
	accessAnnotation := `[{
		"name": "prometheus.infra",
		"kind": "Deployment"
	},
	{
		"name": "client.test-namespace",
		"kind": "Deployment"
	}]`

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"intents.otterize.com/service-name": "client-A",
				"intents.otterize.com/called-by":    accessAnnotation,
			},
		},
	}
	client, ok, err := ParseAccessAnnotations(pod)
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Require().Equal(2, len(client))
	s.Require().Equal("prometheus", client[0].Name)
	s.Require().Equal("infra", client[0].Namespace)
	s.Require().Equal("Deployment", client[0].Kind)

	s.Require().Equal("client", client[1].Name)
	s.Require().Equal("test-namespace", client[1].Namespace)
	s.Require().Equal("Deployment", client[1].Kind)
}

func (s *PodAnnotationParserSuite) TestPodWithoutAnnotation() {
	podName := "pod-1"
	testNamespace := "test-namespace"

	podWithOtherAnnotation := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"intents.otterize.com/service-name": "client-A",
			},
		},
	}
	client, ok, err := ParseAccessAnnotations(podWithOtherAnnotation)
	s.Require().NoError(err)
	s.Require().False(ok)
	s.Require().Equal(0, len(client))

	podWithEmptyObject := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   testNamespace,
			Annotations: map[string]string{},
		},
	}
	client, ok, err = ParseAccessAnnotations(podWithEmptyObject)
	s.Require().NoError(err)
	s.Require().False(ok)
	s.Require().Equal(0, len(client))

	podWithNoAnnotations := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   testNamespace,
			Annotations: nil,
		},
	}
	client, ok, err = ParseAccessAnnotations(podWithNoAnnotations)
	s.Require().NoError(err)
	s.Require().False(ok)
	s.Require().Equal(0, len(client))
}

func TestPodAnnotationParserSuite(t *testing.T) {
	suite.Run(t, new(PodAnnotationParserSuite))
}
