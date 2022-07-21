package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/types"

	"github.com/defenseunicorns/zarf/src/internal/message"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	ZarfNamespace       = "zarf"
	ZarfStateSecretName = "zarf-state"
	ZarfStateDataKey    = "state"
)

// LoadZarfState returns the current zarf/zarf-state secret data or an empty ZarfState
func LoadZarfState() types.ZarfState {
	message.Debug("k8s.LoadZarfState()")

	// The empty state that we will try to fill
	state := types.ZarfState{}

	// Set up the API connection
	secretInterface := getZarfStateInterface()
	message.Note(fmt.Sprintf("!!!jperry!!! the secretInterface: %#v", secretInterface))

	// Try to get the zarf-state secret
	match, err := secretInterface.Get(context.TODO(), ZarfStateSecretName, metav1.GetOptions{})
	if err == nil {
		message.Note("!!!jperry!!! no error when getting the zarf-state secret...")
		_ = json.Unmarshal(match.Data[ZarfStateDataKey], &state)
	} else if err != nil {
		message.Note(fmt.Sprintf("!!!JPERRY!!! Error when getting the zarf-state secret: %#v", err))
	}

	message.Debugf("ZarfState = %s", message.JsonValue(state))

	return state
}

// SaveZarfState takes a given state and makepersists it to the zarf/zarf-state secret
func SaveZarfState(state types.ZarfState) error {
	message.Debugf("k8s.SaveZarfState()")
	message.Debug(message.JsonValue(state))

	// Convert the data back to JSON
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("unable to json-encode the zarf state")
	}

	// Set up the data wrapper
	dataWrapper := make(map[string][]byte)
	dataWrapper[ZarfStateDataKey] = data

	// The secret object
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ZarfStateSecretName,
			Namespace: ZarfNamespace,
			Labels: map[string]string{
				config.ZarfManagedByLabel: "zarf",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: dataWrapper,
	}

	// Attempt to create or replace the secret and return
	if err := ReplaceSecret(secret); err != nil {
		return fmt.Errorf("unable to create the zarf state secret")
	}

	return nil
}

// getZarfStateInterface returns a secret interface for the zarf namespace
func getZarfStateInterface() v1.SecretInterface {
	message.Debug("k8s.getZarfStateInterface()")
	clientSet := getClientset()

	// Get interface for all secrets in the zarf namespace
	return clientSet.CoreV1().Secrets(ZarfNamespace)
}
