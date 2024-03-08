/*
Copyright 2024 The Kubernetes Authors.

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

package worker

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	nfdv1 "sigs.k8s.io/node-feature-discovery-operator/api/v1"
)

//go:generate mockgen -source=worker.go -package=worker -destination=mock_worker.go WorkerAPI

type WorkerAPI interface {
	SetWorkerDaemonsetAsDesired(ctx context.Context, nfdInstance *nfdv1.NodeFeatureDiscovery, workerDS *appsv1.DaemonSet) error
	SetWorkerConfigMapAsDesired(ctx context.Context, nfdInstance *nfdv1.NodeFeatureDiscovery, workerCM *corev1.ConfigMap) error
}

type worker struct {
	scheme *runtime.Scheme
}

func NewWorkerAPI(scheme *runtime.Scheme) WorkerAPI {
	return &worker{
		scheme: scheme,
	}
}

func getImagePullPolicy(nfdInstance *nfdv1.NodeFeatureDiscovery) corev1.PullPolicy {
	if nfdInstance.Spec.Operand.ImagePullPolicy != "" {
		return corev1.PullPolicy(nfdInstance.Spec.Operand.ImagePullPolicy)
	}
	return corev1.PullAlways
}

func getWorkerEnvs(envVars map[string]string) *[]corev1.EnvVar {

	var obj []corev1.EnvVar

	for k, v := range envVars {
		obj = append(obj,
			corev1.EnvVar{
				Name: k,
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: v,
					},
				},
			})
	}

	return &obj
}

func getWorkerAffinity(terms map[string]string) *corev1.Affinity {

	var obj []corev1.NodeSelectorTerm

	for k, v := range terms {
		obj = append(obj, corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      k,
					Operator: corev1.NodeSelectorOperator(v),
				},
			},
		})
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: obj,
			},
		},
	}

}

func getWorkerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		ReadOnlyRootFilesystem:   pointer.Bool(true),
		RunAsNonRoot:             pointer.Bool(true),
		AllowPrivilegeEscalation: pointer.Bool(false),
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

func getWorkerVolumeMounts() *[]corev1.VolumeMount {

	containerVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "host-boot",
			MountPath: "/host-boot",
			ReadOnly:  true,
		},
		{
			Name:      "host-os-release",
			MountPath: "/host-etc/os-release",
			ReadOnly:  true,
		},
		{
			Name:      "host-sys",
			MountPath: "/host-sys",
		},
		{
			Name:      "nfd-worker-config",
			MountPath: "/etc/kubernetes/node-feature-discovery",
		},
		{
			Name:      "nfd-hooks",
			MountPath: "/etc/kubernetes/node-feature-discovery/source.d",
		},
		{
			Name:      "nfd-features",
			MountPath: "/etc/kubernetes/node-feature-discovery/features.d",
		},
	}

	return &containerVolumeMounts
}

func getWorkerVolumes() []corev1.Volume {
	containerVolume := []corev1.Volume{
		{
			Name: "host-dev",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
		{
			Name: "host-boot",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/boot",
				},
			},
		},
		{
			Name: "host-os-release",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/os-release",
				},
			},
		},
		{
			Name: "host-sys",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys",
				},
			},
		},
		{
			Name: "nfd-hooks",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/kubernetes/node-feature-discovery/source.d",
				},
			},
		},
		{
			Name: "nfd-features",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/kubernetes/node-feature-discovery/features.d",
				},
			},
		},
		{
			Name: "nfd-worker-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "nfd-worker"},
					Items: []corev1.KeyToPath{
						{
							Key:  "nfd-worker-conf",
							Path: "nfd-worker.conf",
						},
					},
				},
			},
		},
	}
	return containerVolume
}

func getWorkerLabelsAForApp(name string) map[string]string {
	return map[string]string{"app": name}
}

func (w *worker) SetWorkerDaemonsetAsDesired(ctx context.Context, nfdInstance *nfdv1.NodeFeatureDiscovery, workerDS *appsv1.DaemonSet) error {
	workerDS.ObjectMeta.Labels = map[string]string{"app": "nfd"}

	envVars := map[string]string{"NODE_NAME": "spec.nodeName"}

	nodeaffinities := map[string]string{
		"node-role.kubernetes.io/master": "DoesNotExist",
		"node-role.kubernetes.io/node":   "Exists",
	}

	workerDS.Spec = appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: getWorkerLabelsAForApp("nfd-worker"),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: getWorkerLabelsAForApp("nfd-worker"),
			},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Operator: "Exists",
						Effect:   "NoSchedule",
					},
				},
				Affinity: getWorkerAffinity(nodeaffinities),

				ServiceAccountName: "nfd-worker",
				DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
				Containers: []corev1.Container{
					{
						Env:             *getWorkerEnvs(envVars),
						Image:           nfdInstance.Spec.Operand.ImagePath(),
						Name:            "nfd-worker",
						Command:         []string{"nfd-worker"},
						Args:            []string{"--server=nfd-master:$(NFD_MASTER_SERVICE_PORT)"},
						VolumeMounts:    *getWorkerVolumeMounts(),
						ImagePullPolicy: getImagePullPolicy(nfdInstance),
						SecurityContext: getWorkerSecurityContext(),
					},
				},
				Volumes: getWorkerVolumes(),
			},
		},
	}
	return controllerutil.SetControllerReference(nfdInstance, workerDS, w.scheme)
}

func (w *worker) SetWorkerConfigMapAsDesired(ctx context.Context, nfdInstance *nfdv1.NodeFeatureDiscovery, workerCM *corev1.ConfigMap) error {

	workerCM.Data = map[string]string{"nfd-worker-conf": nfdInstance.Spec.WorkerConfig.ConfigData}

	return controllerutil.SetControllerReference(nfdInstance, workerCM, w.scheme)
}
