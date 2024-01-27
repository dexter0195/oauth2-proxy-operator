package proxy

import (
	"fmt"
	"os"
)

// imageForOAuth2Proxy gets the Operand image which is managed by this controller
// from the OAUTH2PROXY_IMAGE environment variable defined in the config/manager/manager.yaml
func ImageForOAuth2Proxy() (string, error) {
	var imageEnvVar = "OAUTH2PROXY_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}
