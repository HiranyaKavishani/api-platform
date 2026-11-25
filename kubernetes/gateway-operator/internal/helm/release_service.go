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
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// ErrReleaseNotFound is returned by ReleaseService when the release does not exist.
var ErrReleaseNotFound = errors.New("helm release not found")

// componentLabel is the label the gateway chart puts on each component's objects.
const componentLabel = "app.kubernetes.io/component"

// ManifestService is a Service as a Helm release rendered it: the name Helm
// created it under and the pod selector it was given.
type ManifestService struct {
	Name     string
	Selector map[string]string
}

// ReleaseService returns the Service of the given component (for example
// "controller") from the manifest Helm recorded for releaseName in namespace.
//
// Labels on live objects can be set by anyone who can create a Service in the
// namespace; the release record is written only by Helm. Reading the name from
// it lets a caller look the Service up by the name Helm created, rather than by
// labels.
func (c *Client) ReleaseService(namespace, releaseName, component string) (*ManifestService, error) {
	actionConfig, err := c.newActionConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("helm action config: %w", err)
	}
	rel, err := action.NewGet(actionConfig).Run(releaseName)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) || errors.Is(err, driver.ErrNoDeployedReleases) {
			return nil, fmt.Errorf("%w: %s in namespace %s", ErrReleaseNotFound, releaseName, namespace)
		}
		return nil, fmt.Errorf("helm get release %q in namespace %q: %w", releaseName, namespace, err)
	}
	if rel == nil {
		return nil, fmt.Errorf("%w: %s in namespace %s", ErrReleaseNotFound, releaseName, namespace)
	}
	return FindManifestService(rel.Manifest, component)
}

// FindManifestService returns the one Service in a rendered manifest whose
// component label is component. It is an error if there is none or more than one.
func FindManifestService(manifest, component string) (*ManifestService, error) {
	type service struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name   string            `yaml:"name"`
			Labels map[string]string `yaml:"labels"`
		} `yaml:"metadata"`
		Spec struct {
			Selector map[string]string `yaml:"selector"`
		} `yaml:"spec"`
	}

	var found []*ManifestService
	for _, doc := range releaseutil.SplitManifests(manifest) {
		var s service
		if err := yaml.Unmarshal([]byte(doc), &s); err != nil {
			return nil, fmt.Errorf("parse release manifest: %w", err)
		}
		if s.Kind != "Service" || s.Metadata.Labels[componentLabel] != component || s.Metadata.Name == "" {
			continue
		}
		found = append(found, &ManifestService{Name: s.Metadata.Name, Selector: s.Spec.Selector})
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("release manifest has no %s Service", component)
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("release manifest has %d %s Services, expected one", len(found), component)
	}
}
