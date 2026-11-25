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

package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const releaseManifest = `---
# Source: gateway/templates/gateway/controller/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: gw-gateway-controller
  labels:
    app.kubernetes.io/instance: gw-gateway
    app.kubernetes.io/component: controller
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/instance: gw-gateway
    app.kubernetes.io/component: controller
  ports:
    - name: rest
      port: 9090
---
# Source: gateway/templates/gateway/runtime/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: gw-gateway-runtime
  labels:
    app.kubernetes.io/component: gateway-runtime
spec:
  selector:
    app.kubernetes.io/component: gateway-runtime
---
# Source: gateway/templates/gateway/controller/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gw-gateway-controller
  labels:
    app.kubernetes.io/component: controller
`

func TestFindManifestService(t *testing.T) {
	got, err := FindManifestService(releaseManifest, "controller")
	require.NoError(t, err)
	require.Equal(t, "gw-gateway-controller", got.Name)
	require.Equal(t, map[string]string{
		"app.kubernetes.io/instance":  "gw-gateway",
		"app.kubernetes.io/component": "controller",
	}, got.Selector)

	runtime, err := FindManifestService(releaseManifest, "gateway-runtime")
	require.NoError(t, err)
	require.Equal(t, "gw-gateway-runtime", runtime.Name)
}

func TestFindManifestService_NoMatch(t *testing.T) {
	_, err := FindManifestService(releaseManifest, "policy-engine")
	require.ErrorContains(t, err, "no policy-engine Service")

	_, err = FindManifestService("", "controller")
	require.ErrorContains(t, err, "no controller Service")
}

func TestFindManifestService_MoreThanOne(t *testing.T) {
	twice := releaseManifest + `---
apiVersion: v1
kind: Service
metadata:
  name: second-controller
  labels:
    app.kubernetes.io/component: controller
`
	_, err := FindManifestService(twice, "controller")
	require.ErrorContains(t, err, "2 controller Services")
}
