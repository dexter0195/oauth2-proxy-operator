package proxy

import (
	"strings"
)

// labelsForOAuth2Proxy returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func LabelsForOAuth2Proxy(name string) map[string]string {
	var imageTag string
	image, err := ImageForOAuth2Proxy()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{"app.kubernetes.io/name": "OAuth2Proxy",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "oauth2-proxy-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}
