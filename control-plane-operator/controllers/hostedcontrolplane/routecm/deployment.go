package routecm

import (
	"fmt"
	"path"

	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"github.com/openshift/hypershift/control-plane-operator/controllers/hostedcontrolplane/manifests"
	"github.com/openshift/hypershift/support/config"
	"github.com/openshift/hypershift/support/util"
)

const (
	configHashAnnotation = "openshift-route-controller-manager.hypershift.openshift.io/config-hash"

	servingPort int32 = 8443
	configKey         = "config.yaml"
)

var (
	volumeMounts = util.PodVolumeMounts{
		routeOCMContainerMain().Name: {
			routeOCMVolumeConfig().Name:      "/etc/kubernetes/config",
			routeOCMVolumeServingCert().Name: "/etc/kubernetes/certs",
			routeOCMVolumeKubeconfig().Name:  "/etc/kubernetes/secrets/svc-kubeconfig",
		},
	}
)

func openShiftRouteControllerManagerLabels() map[string]string {
	return map[string]string{
		"app":                         "openshift-route-controller-manager",
		hyperv1.ControlPlaneComponent: "openshift-route-controller-manager",
	}
}

func ReconcileDeployment(deployment *appsv1.Deployment, image string, config *corev1.ConfigMap, deploymentConfig config.DeploymentConfig) error {
	configBytes, ok := config.Data[configKey]
	if !ok {
		return fmt.Errorf("openshift controller manager configuration is not expected to be empty")
	}
	configHash := util.ComputeHash(configBytes)

	// preserve existing resource requirements for main OCM container
	mainContainer := util.FindContainer(routeOCMContainerMain().Name, deployment.Spec.Template.Spec.Containers)
	if mainContainer != nil {
		deploymentConfig.SetContainerResourcesIfPresent(mainContainer)
	}

	maxSurge := intstr.FromInt(0)
	maxUnavailable := intstr.FromInt(1)
	deployment.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	deployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
		MaxSurge:       &maxSurge,
		MaxUnavailable: &maxUnavailable,
	}
	if deployment.Spec.Selector == nil {
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: openShiftRouteControllerManagerLabels(),
		}
	}
	deployment.Spec.Template.ObjectMeta.Labels = openShiftRouteControllerManagerLabels()
	deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{
		configHashAnnotation: configHash,
	}
	deployment.Spec.Template.Spec.AutomountServiceAccountToken = pointer.BoolPtr(false)
	deployment.Spec.Template.Spec.Containers = []corev1.Container{
		util.BuildContainer(routeOCMContainerMain(), buildRouteOCMContainerMain(image)),
	}
	deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
		util.BuildVolume(routeOCMVolumeConfig(), buildRouteOCMVolumeConfig),
		util.BuildVolume(routeOCMVolumeServingCert(), buildRouteOCMVolumeServingCert),
		util.BuildVolume(routeOCMVolumeKubeconfig(), buildRouteOCMVolumeKubeconfig),
	}
	deploymentConfig.ApplyTo(deployment)
	return nil
}

func routeOCMContainerMain() *corev1.Container {
	return &corev1.Container{
		Name: "openshift-controller-manager",
	}
}

func buildRouteOCMContainerMain(image string) func(*corev1.Container) {
	return func(c *corev1.Container) {
		c.Image = image
		c.Command = []string{"openshift-controller-manager"}
		c.Args = []string{
			"route-controller-manager-start",
			"--config",
			path.Join(volumeMounts.Path(c.Name, routeOCMVolumeConfig().Name), configKey),
		}
		c.VolumeMounts = volumeMounts.ContainerMounts(c.Name)
		c.Ports = []corev1.ContainerPort{
			{
				Name:          "https",
				ContainerPort: servingPort,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}
}

func routeOCMVolumeConfig() *corev1.Volume {
	return &corev1.Volume{
		Name: "config",
	}
}

func buildRouteOCMVolumeConfig(v *corev1.Volume) {
	v.ConfigMap = &corev1.ConfigMapVolumeSource{}
	v.ConfigMap.Name = manifests.OpenShiftControllerManagerConfig("").Name
}

func routeOCMVolumeKubeconfig() *corev1.Volume {
	return &corev1.Volume{
		Name: "kubeconfig",
	}
}

func buildRouteOCMVolumeKubeconfig(v *corev1.Volume) {
	v.Secret = &corev1.SecretVolumeSource{}
	v.Secret.SecretName = manifests.KASServiceKubeconfigSecret("").Name
}

func routeOCMVolumeServingCert() *corev1.Volume {
	return &corev1.Volume{
		Name: "serving-cert",
	}
}

func buildRouteOCMVolumeServingCert(v *corev1.Volume) {
	v.Secret = &corev1.SecretVolumeSource{}
	v.Secret.SecretName = manifests.OpenShiftRouteControllerManagerCertSecret("").Name
}
