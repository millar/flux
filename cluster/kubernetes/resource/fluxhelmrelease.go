package resource

import (
	"fmt"

	ifv1 "github.com/weaveworks/flux/apis/helm.integrations.flux.weave.works/v1alpha"
	"github.com/weaveworks/flux/image"
	"github.com/weaveworks/flux/resource"
	"k8s.io/helm/pkg/chartutil"
)

type FluxHelmRelease struct {
	baseObject
	Spec ifv1.FluxHelmReleaseSpec
}

func (fhr FluxHelmRelease) Containers() []resource.Container {
	// process the Spec.Values section and construct containers
	containers, err := CreateContainers(fhr)
	if err != nil {
		// log ?
	}
	return containers
}

func CreateContainers(fhr FluxHelmRelease) ([]resource.Container, error) {
	containers := []resource.Container{}

	values := fhr.Spec.Values
	if len(values) == 0 {
		return containers, nil
	}
	imgInfo, ok := values["image"]

	// image info appears on the top level, so is associated directly with the chart
	if ok {
		imageRef, err := processImageInfo(values, imgInfo)
		if err != nil {
			return nil, err
		}
		containers = append(containers, resource.Container{Name: fhr.Spec.ChartGitPath, Image: imageRef})
		return containers, nil
	}

	// no top key is an image parameter =>
	// image is potentially provided nested within the map value of the top key(s)
	for param, value := range values {
		cName, imageRef, err := fhr.findImage(param, value)
		if err != nil {
			return nil, err
		}

		// an empty cName means no image info was found under this parameter
		if cName != "" {
			containers = append(containers, resource.Container{Name: cName, Image: imageRef})
		}
	}

	return []resource.Container{}, nil
}

func processImageInfo(values map[string]interface{}, value interface{}) (image.Ref, error) {
	var ref image.Ref
	var err error

	switch value.(type) {
	case string:
		val := value.(string)
		ref, err = processImageString(values, val)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil

	case map[string]string:
		// image:
		// 			registry: docker.io   (sometimes missing)
		// 			repository: bitnami/mariadb
		// 			tag: 10.1.32					(sometimes version)
		val := value.(map[string]string)
		ref, err = processImageMap(val)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil

	default:
		return image.Ref{}, image.ErrMalformedImageID
	}
}

// findImage tries to find image info among the nested Spec.Values
// nested image info examples ---------------------------------------------------
// 		controller:
// 			image:
// 				repository: quay.io/kubernetes-ingress-controller/nginx-ingress-controller
// 				tag: "0.12.0"

// 		jupyter:
// 			image:
// 				repository: "daskdev/dask-notebook"
// 				tag: "0.17.1"

// 		zeppelin:
// 			image: dylanmei/zeppelin:0.7.2

// 		artifactory:
//   		name: artifactory
//  		replicaCount: 1
//  		image:
//    	  # repository: "docker.bintray.io/jfrog/artifactory-oss"
//   		  repository: "docker.bintray.io/jfrog/artifactory-pro"
//  		  version: 5.9.1
//   		  pullPolicy: IfNotPresent
func (fhr FluxHelmRelease) findImage(param string, value interface{}) (string, image.Ref, error) {
	fhrName := fhr.Meta.Name

	switch value.(type) {
	case string:
		return "", image.Ref{}, nil
	case map[string]interface{}:
		val := value.(map[string]interface{})

		cName := fhrName
		if cn, ok := val["name"]; ok {
			if cns, ok := cn.(string); ok {
				cName = cns
			}
		}
		refP, err := processImageData(val)
		if err != nil {
			return "", image.Ref{}, err
		}
		return cName, refP, nil
	default:
		return "", image.Ref{}, nil
	}
}

func processImageString(values chartutil.Values, val string) (image.Ref, error) {
	if t, ok := values["imageTag"]; ok {
		val = fmt.Sprintf("%s:%s", val, t)
	} else if t, ok := values["tag"]; ok {
		val = fmt.Sprintf("%s:%s", val, t)
	}
	ref, err := image.ParseRef(val)
	if err != nil {
		return image.Ref{}, err
	}
	// returning chart to be the container name
	return ref, nil
}

func processImageMap(val map[string]string) (image.Ref, error) {
	var ref image.Ref
	var err error

	i, iOk := val["repository"]
	if !iOk {
		return image.Ref{}, image.ErrMalformedImageID
	}

	d, dOk := val["registry"]
	t, tOk := val["tag"]

	if !dOk {
		if tOk {
			i = fmt.Sprintf("%s:%s", i, t)
		}
		ref, err = image.ParseRef(i)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil
	}
	if !tOk {
		if dOk {
			i = fmt.Sprintf("%s/%s", d, i)
		}
		ref, err = image.ParseRef(i)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil
	}

	return image.Ref{
		Name: image.Name{
			Domain: d,
			Image:  i,
		},
		Tag: t,
	}, nil
}

// processImageData processes value of the image parameter, if it exists
func processImageData(value map[string]interface{}) (image.Ref, error) {
	iVal, ok := value["image"]
	if !ok {
		return image.Ref{}, nil
	}

	var ref image.Ref
	var err error

	switch iVal.(type) {
	case string:
		val := iVal.(string)
		ref, err = processImageString(value, val)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil

	case map[string]string:
		// image:
		// 			registry: docker.io   (sometimes missing)
		// 			repository: bitnami/mariadb
		// 			tag: 10.1.32					(sometimes version)
		val := iVal.(map[string]string)

		ref, err = processImageMap(val)
		if err != nil {
			return image.Ref{}, err
		}
		return ref, nil
	default:
		return image.Ref{}, nil
	}
}
