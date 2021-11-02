package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	admission "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"log"
	"net/http"
)

var (
	podResource           = metav1.GroupVersionResource{Version: "v1", Resource: "pods"}
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

// WebHookParameters webhook相关参数
type WebHookParameters struct {
	port              int
	certFile          string
	keyFile           string
	sidecarConfigFile string
}

// SidecarConfig 配置文件
type SidecarConfig struct {
	Containers []corev1.Container `yaml:"containers"`
	Volumes    []corev1.Volume    `yaml:"volumes"`
}

// WebhookServer
type WebhookServer struct {
	sidecarConfig *SidecarConfig
	httpServer    *http.Server
}

func loadSidecarConfig(sidecarfile string) (*SidecarConfig, error) {
	data, err := ioutil.ReadFile(sidecarfile)
	if err != nil {
		return nil, err
	}

	var sidecarConfig SidecarConfig

	err = yaml.Unmarshal(data, &sidecarConfig)
	if err != nil {
		return nil, err
	}

	return &sidecarConfig, nil
}

func addNginxSidecar(req *admission.AdmissionRequest, config *SidecarConfig, annotations map[string]string) ([]patchOperation, error) {
	if req.Resource != podResource {
		log.Printf("expect resource to be %s", podResource)
		return nil, nil
	}

	// 解析pod
	raw := req.Object.Raw
	pod := corev1.Pod{}
	if _, _, err := universalDeserializer.Decode(raw, nil, &pod); err != nil {
		return nil, fmt.Errorf("不能解析pod对象: %v\n", err)
	}
	var patches []patchOperation
	patches = append(patches, addContainer(pod.Spec.Containers, config.Containers, "/spec/containers")...)
	patches = append(patches, addVolume(pod.Spec.Volumes, config.Volumes, "/spec/volumes")...)
	patches = append(patches, updateAnnotation(pod.Annotations, annotations)...)
	return patches, nil
}

func addContainer(target, added []corev1.Container, basePath string) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Container{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func addVolume(target, added []corev1.Volume, basePath string) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Volume{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func updateAnnotation(target map[string]string, added map[string]string) (patch []patchOperation) {
	for key, value := range added {
		if target == nil || target[key] == "" {
			target = map[string]string{}
			patch = append(patch, patchOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					key: value,
				},
			})
		} else {
			patch = append(patch, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + key,
				Value: value,
			})
		}
	}
	return patch
}
