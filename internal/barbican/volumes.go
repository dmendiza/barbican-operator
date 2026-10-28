package barbican

import (
	"fmt"
	"slices"

	barbicanv1beta1 "github.com/openstack-k8s-operators/barbican-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	corev1 "k8s.io/api/core/v1"
)

var (
	configMode int32 = 0440
	scriptMode int32 = 0740
)

// GetVolumes - service volumes
func GetVolumes(name string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: ConfigVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  name + "-config-data",
				},
			},
		},
	}
}

// GetVolumeMounts - general VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      ConfigVolume,
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
}

// GetRunHttpdVolume - Returns the emptyDir Volume used for the httpd PID file
func GetRunHttpdVolume() corev1.Volume {
	return volume.WritableDirVolume(volume.RunHttpdVolumeName)
}

// GetVarLogHttpdVolume - Returns the emptyDir Volume used for httpd's own logs
func GetVarLogHttpdVolume() corev1.Volume {
	return volume.WritableDirVolume(volume.VarLogHttpdVolumeName)
}

// GetAPIHttpdVolumeMounts - Returns the VolumeMounts needed only by the
// barbican-api httpd/WSGI container, mounted at their final destinations
// (httpd.conf, mime.conf, ssl.conf, the WSGI vhost config and the WSGI
// entry script itself), plus the run-httpd/var-log-httpd emptyDir mounts.
func GetAPIHttpdVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      ConfigVolume,
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/etc/httpd/conf.modules.d/mime.conf",
			SubPath:   "mime.conf",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/etc/httpd/conf.d/10-barbican_wsgi_main.conf",
			SubPath:   "10-barbican_wsgi_main.conf",
			ReadOnly:  true,
		},
		{
			Name:      ConfigVolume,
			MountPath: "/var/www/cgi-bin/barbican/main",
			SubPath:   "main",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
		volume.WritableDirVolumeMount(volume.VarLogHttpdVolumeName, volume.VarLogHttpdMountPath),
	}
}

// GetLogVolumeMount - Returns the VolumeMount used for logging purposes
func GetLogVolumeMount() corev1.VolumeMount {
	return volume.WritableDirVolumeMount(LogVolume, "/var/log/barbican")
}

// GetLogVolume - Returns the Volume used for logging purposes
func GetLogVolume() corev1.Volume {
	return volume.WritableDirVolume(LogVolume)
}

// GetScriptVolumeMount - Returns the VolumeMount for scripts
func GetScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      ScriptVolume,
		MountPath: ScriptMountPoint,
		ReadOnly:  true,
	}
}

// GetScriptVolume - Return the Volume for scripts
func GetScriptVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: ScriptVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &scriptMode,
				SecretName:  secretName,
			},
		},
	}
}

// getClientDataVolume returns the Volume exposing a secret store's client data
// Secret (HSM vendor client files, ...) for the stores that mount it directly.
func getClientDataVolume(volumeName string, clientData barbicanv1beta1.ClientDataSpec) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &configMode,
				SecretName:  clientData.ClientDataSecret,
			},
		},
	}
}

// getClientDataVolumeMount returns the VolumeMount for a secret store's client
// data, mounted directly at its ClientDataPath -- the final location the
// backend client library expects, rather than a staging path kolla used to
// copy from.
func getClientDataVolumeMount(volumeName string, clientData barbicanv1beta1.ClientDataSpec, defaultPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: clientData.PathOrDefault(defaultPath),
		ReadOnly:  true,
	}
}

// GetClientDataVolumes returns the client data Volumes and VolumeMounts for
// every enabled secret store that mounts files from a Secret. Each store gets
// its own volume name and mount path, so stores enabled together do not
// collide.
func GetClientDataVolumes(template *barbicanv1beta1.BarbicanTemplate) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount

	if slices.Contains(template.EnabledSecretStores, barbicanv1beta1.SecretStorePKCS11) && template.PKCS11 != nil {
		volumes = append(volumes, getClientDataVolume(PKCS11ClientDataVolume, template.PKCS11.ClientDataSpec))
		mounts = append(mounts, getClientDataVolumeMount(
			PKCS11ClientDataVolume, template.PKCS11.ClientDataSpec, barbicanv1beta1.DefaultPKCS11ClientDataPath))
	}

	if slices.Contains(template.EnabledSecretStores, barbicanv1beta1.SecretStoreKMIP) && template.KMIP != nil {
		// The KMIP client data cannot be mounted from its Secret directly --
		// see GetKMIPClientDataInitContainer. The Secret is mounted at a
		// staging path for the init container only, and the service
		// containers get the emptyDir the init container copies into.
		volumes = append(volumes,
			getKMIPClientDataSourceVolume(template.KMIP.ClientDataSpec),
			volume.WritableDirVolume(KMIPClientDataVolume))
		mounts = append(mounts, corev1.VolumeMount{
			Name:      KMIPClientDataVolume,
			MountPath: template.KMIP.PathOrDefault(barbicanv1beta1.DefaultKMIPClientDataPath),
			ReadOnly:  true,
		})
	}

	return volumes, mounts
}

// getKMIPClientDataSourceVolume returns the Volume exposing the KMIP client
// data Secret at the staging path the init container copies from.
func getKMIPClientDataSourceVolume(clientData barbicanv1beta1.ClientDataSpec) corev1.Volume {
	return corev1.Volume{
		Name: KMIPClientDataSourceVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &configMode,
				SecretName:  clientData.ClientDataSecret,
			},
		},
	}
}

// kmipClientDataScript copies the KMIP client data from the Secret mounted at
// $1 into the emptyDir mounted at $2, leaving every file mode 0400 and owned
// by the barbican user. cp -f so that an init container re-run over a
// surviving emptyDir can overwrite the unwritable files it wrote last time,
// and cp -L because every entry in a Secret mount is a symlink into ..data.
const kmipClientDataScript = `set -eu
cp -fL "$1"/* "$2"/
chmod 0400 "$2"/*
`

// GetKMIPClientDataInitContainer returns the init container that stages the
// KMIP client data out of its Secret and into the emptyDir mounted at
// ClientDataPath.
//
// The copy exists purely to get the key file to mode 0400: barbican's KMIP
// plugin refuses to start unless [kmip_plugin] keyfile is a regular file with
// exactly those permissions (KMIPSecretStore._validate_keyfile_permissions).
// A Secret volume cannot provide that -- the pods set an fsGroup so that the
// unprivileged barbican user can read mounted Secrets, and the kubelet
// implements that by ORing 0440 onto every file in the volume, so even
// DefaultMode 0400 lands on disk as 0440 and barbican rejects it. Files the
// init container writes into an emptyDir afterwards are not touched by that
// ownership pass, so they keep both the 0400 mode and barbican's UID.
//
// The operator does not know which file in the Secret is the key -- the
// [kmip_plugin] file names come from CustomServiceConfig -- so every file is
// staged as 0400. The certificate and CA bundle are read by the same barbican
// user, so owner-only read is enough for them too.
func GetKMIPClientDataInitContainer(
	template *barbicanv1beta1.BarbicanTemplate,
	image string,
	resources corev1.ResourceRequirements,
) corev1.Container {
	clientDataPath := template.KMIP.PathOrDefault(barbicanv1beta1.DefaultKMIPClientDataPath)
	return corev1.Container{
		Name:  KMIPClientDataInitContainer,
		Image: image,
		Command: []string{
			"/bin/sh", "-c", kmipClientDataScript, KMIPClientDataInitContainer,
		},
		Args:            []string{KMIPClientDataSourcePath, clientDataPath},
		SecurityContext: pod.RestrictiveSecurityContext(users.BarbicanUID, users.BarbicanGID),
		Resources:       resources,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      KMIPClientDataSourceVolume,
				MountPath: KMIPClientDataSourcePath,
				ReadOnly:  true,
			},
			{
				Name:      KMIPClientDataVolume,
				MountPath: clientDataPath,
			},
		},
	}
}

// GetClientDataInitContainers returns the init containers needed to stage the
// client data of the enabled secret stores. Only KMIP needs one; the PKCS11
// vendor client data is mounted from its Secret directly.
func GetClientDataInitContainers(
	template *barbicanv1beta1.BarbicanTemplate,
	image string,
	resources corev1.ResourceRequirements,
) []corev1.Container {
	var initContainers []corev1.Container

	if slices.Contains(template.EnabledSecretStores, barbicanv1beta1.SecretStoreKMIP) && template.KMIP != nil {
		initContainers = append(initContainers, GetKMIPClientDataInitContainer(template, image, resources))
	}

	return initContainers
}

// GetCustomConfigVolume - service custom config volume
func GetCustomConfigVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: CustomConfigVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &configMode,
				SecretName:  name + "-config-data",
			},
		},
	}
}

// GetCustomConfigVolumeMount - service custom config volume mount
func GetCustomConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      CustomConfigVolume,
		MountPath: CustomConfigMountPoint,
		ReadOnly:  true,
	}
}

// GetConfigOverwriteVolumeMounts returns SubPath volume mounts that place
// each defaultConfigOverwrite key as an individual file under /etc/barbican/.
// SubPath mounts are used instead of a directory mount so that existing files
// in /etc/barbican/ (barbican.conf, api-paste.ini, etc.) are not shadowed.
// The overwrite data lives in the same config-data-custom Secret (merged via
// CustomData).
//
// Backwards compatibility: the overwrite keys also remain accessible at
// /etc/barbican/barbican.conf.d/<key> because the config-data-custom Secret
// is already directory-mounted at /etc/barbican/barbican.conf.d/ by
// GetCustomConfigVolumeMount. This preserves customer workarounds where e.g.
// policy.yaml was referenced via customServiceConfig as:
//
//	[oslo_policy]
//	policy_file = /etc/barbican/barbican.conf.d/policy.yaml
func GetConfigOverwriteVolumeMounts(overwriteKeys []string) []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(overwriteKeys))
	sorted := make([]string, len(overwriteKeys))
	copy(sorted, overwriteKeys)
	slices.Sort(sorted)
	for _, key := range sorted {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      CustomConfigVolume,
			MountPath: fmt.Sprintf("%s/%s", ConfigOverwriteBasePath, key),
			SubPath:   key,
			ReadOnly:  true,
		})
	}
	return mounts
}

// GetExtraVolumes returns the volumes and volume mounts derived from the
// ExtraMounts spec, filtered by the given propagation types.
func GetExtraVolumes(extraMounts []barbicanv1beta1.BarbicanExtraVolMounts, svc []storage.PropagationType) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount
	for _, exv := range extraMounts {
		for _, vol := range exv.Propagate(svc) {
			for _, v := range vol.Volumes {
				volumeSource, _ := v.ToCoreVolumeSource()
				volumes = append(volumes, corev1.Volume{
					Name:         v.Name,
					VolumeSource: *volumeSource,
				})
			}
			mounts = append(mounts, vol.Mounts...)
		}
	}
	return volumes, mounts
}

// GetOsloConfigVolumes returns a Volume and matching VolumeMount that expose
// only the two oslo.config snippet files a barbican-manage invocation needs
// (DefaultsConfigFileName and CustomConfigFileName) under
// /etc/barbican/barbican.conf.d, selected out of the top-level barbican
// config-data Secret via Items.
//
// Unlike the individual Barbican services -- which mount the whole config-data
// Secret so httpd and the WSGI app can read their own files -- Jobs that run
// barbican-manage (DbSyncJob, PKCS11PrepJob) must NOT expose the Apache config
// files (httpd.conf, ssl.conf, mime.conf, 10-barbican_wsgi_main.conf) that also
// live in that Secret: barbican-manage relies on oslo.config defaults, which
// auto-load the barbican.conf.d config-dir and parse every *.conf file as INI.
// Those Apache files are not valid INI, so a full directory mount makes
// barbican-manage fail on startup. Selecting only the snippet files avoids this.
//
// volumeName lets the caller give the volume a pod-unique name.
func GetOsloConfigVolumes(name, volumeName string) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{
		{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  name + "-config-data",
					Items: []corev1.KeyToPath{
						{
							Key:  DefaultsConfigFileName,
							Path: DefaultsConfigFileName,
						},
						{
							Key:  CustomConfigFileName,
							Path: CustomConfigFileName,
						},
					},
				},
			},
		},
	}

	mounts := []corev1.VolumeMount{
		{
			Name:      volumeName,
			MountPath: CustomConfigMountPoint,
			ReadOnly:  true,
		},
	}
	return volumes, mounts
}
