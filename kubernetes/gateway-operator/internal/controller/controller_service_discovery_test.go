/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/wso2/api-platform/kubernetes/gateway-operator/internal/helm"
)

// fakeReleaseServices answers ReleaseService from a fixed map, as the Helm release record would.
type fakeReleaseServices struct {
	services map[string]*helm.ManifestService // key: namespace/release/component
	err      error
}

func (f *fakeReleaseServices) ReleaseService(namespace, releaseName, component string) (*helm.ManifestService, error) {
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.services[namespace+"/"+releaseName+"/"+component]
	if !ok {
		return nil, helm.ErrReleaseNotFound
	}
	return s, nil
}

func TestDiscoverControllerService(t *testing.T) {
	const (
		gatewayName = "gw1"
		namespace   = "infra"
	)
	release := helm.GetReleaseName(gatewayName)
	labels := map[string]string{
		"app.kubernetes.io/instance":  release,
		"app.kubernetes.io/component": "controller",
	}
	helmAnnotations := map[string]string{
		helmReleaseNameAnnotation:      release,
		helmReleaseNamespaceAnnotation: namespace,
	}
	genuineName := release + "-controller"
	rendered := &fakeReleaseServices{services: map[string]*helm.ManifestService{
		namespace + "/" + release + "/controller": {Name: genuineName, Selector: labels},
	}}

	genuine := func() *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: genuineName, Namespace: namespace,
				Labels: labels, Annotations: helmAnnotations,
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: labels,
				Ports:    []corev1.ServicePort{{Name: "rest", Port: 9090}},
			},
		}
	}
	// A Service someone else created with the same labels, sorting before the genuine one.
	decoy := func() *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "a-decoy", Namespace: namespace, Labels: labels},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "attacker.example.com",
			},
		}
	}

	tests := []struct {
		name    string
		src     releaseServiceSource
		objs    []client.Object
		want    string
		wantErr string
	}{
		{
			name: "the release's own Service is used",
			src:  rendered,
			objs: []client.Object{genuine()},
			want: genuineName,
		},
		{
			name: "a labeled decoy is ignored",
			src:  rendered,
			objs: []client.Object{decoy(), genuine()},
			want: genuineName,
		},
		{
			name:    "only a labeled decoy: not registered",
			src:     rendered,
			objs:    []client.Object{decoy()},
			wantErr: "get gateway controller Service",
		},
		{
			name: "ExternalName under the release's name is refused",
			src:  rendered,
			objs: []client.Object{func() client.Object {
				s := genuine()
				s.Spec.Type = corev1.ServiceTypeExternalName
				s.Spec.ExternalName = "attacker.example.com"
				return s
			}()},
			wantErr: "ExternalName",
		},
		{
			name: "a Service not owned by the release is refused",
			src:  rendered,
			objs: []client.Object{func() client.Object {
				s := genuine()
				s.Annotations = map[string]string{helmReleaseNameAnnotation: "other", helmReleaseNamespaceAnnotation: namespace}
				return s
			}()},
			wantErr: "not owned by Helm release",
		},
		{
			name: "a Service without Helm's annotations is refused",
			src:  rendered,
			objs: []client.Object{func() client.Object {
				s := genuine()
				s.Annotations = nil
				return s
			}()},
			wantErr: "not owned by Helm release",
		},
		{
			name: "a Service selecting other pods is refused",
			src:  rendered,
			objs: []client.Object{func() client.Object {
				s := genuine()
				s.Spec.Selector = map[string]string{"app": "attacker"}
				return s
			}()},
			wantErr: "selects other pods",
		},
		{
			name:    "no release record: not registered",
			src:     &fakeReleaseServices{},
			objs:    []client.Object{genuine()},
			wantErr: "find the gateway controller Service in Helm release",
		},
		{
			name:    "release read fails: not registered",
			src:     &fakeReleaseServices{err: errors.New("storage unavailable")},
			objs:    []client.Object{genuine()},
			wantErr: "storage unavailable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objs...).Build()

			svc, port, err := discoverControllerService(context.Background(), cl, tc.src, gatewayName, namespace)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, svc)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, svc.Name)
			require.Equal(t, int32(9090), port)
		})
	}
}

func TestControllerRESTPort(t *testing.T) {
	svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
		{Name: "metrics", Port: 9091},
		{Name: "rest", Port: 19090},
	}}}
	require.Equal(t, int32(19090), controllerRESTPort(svc))
	require.Equal(t, int32(9090), controllerRESTPort(&corev1.Service{}))
}
