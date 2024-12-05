/*
Copyright 2024.

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
// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"

	v1alpha1 "github.com/mNi-Cloud/operator/api/v1alpha1"
	scheme "github.com/mNi-Cloud/operator/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// ComponentsGetter has a method to return a ComponentInterface.
// A group's client should implement this interface.
type ComponentsGetter interface {
	Components() ComponentInterface
}

// ComponentInterface has methods to work with Component resources.
type ComponentInterface interface {
	Create(ctx context.Context, component *v1alpha1.Component, opts v1.CreateOptions) (*v1alpha1.Component, error)
	Update(ctx context.Context, component *v1alpha1.Component, opts v1.UpdateOptions) (*v1alpha1.Component, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, component *v1alpha1.Component, opts v1.UpdateOptions) (*v1alpha1.Component, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.Component, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ComponentList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Component, err error)
	ComponentExpansion
}

// components implements ComponentInterface
type components struct {
	*gentype.ClientWithList[*v1alpha1.Component, *v1alpha1.ComponentList]
}

// newComponents returns a Components
func newComponents(c *ApiV1alpha1Client) *components {
	return &components{
		gentype.NewClientWithList[*v1alpha1.Component, *v1alpha1.ComponentList](
			"components",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *v1alpha1.Component { return &v1alpha1.Component{} },
			func() *v1alpha1.ComponentList { return &v1alpha1.ComponentList{} }),
	}
}
