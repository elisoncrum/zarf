package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/internal/agent/operations"
	"github.com/defenseunicorns/zarf/src/internal/git"
	"github.com/defenseunicorns/zarf/src/internal/k8s"
	"github.com/defenseunicorns/zarf/src/internal/message"
	v1 "k8s.io/api/admission/v1"
)

type SecretRef struct {
	Name string `json:"name"`
}

type GenericGitRepo struct {
	Spec struct {
		URL       string    `json:"url"`
		SecretRef SecretRef `json:"secretRef,omitempty"`
	}
}

// NewGitRepositoryMutationHook creates a new instance of the git repo mutation hook
func NewGitRepositoryMutationHook() operations.Hook {
	message.Debug("hooks.NewGitRepositoryMutationHook()")
	return operations.Hook{
		Create: mutateGitRepository,
		Update: mutateGitRepository,
	}
}

func mutateGitRepository(r *v1.AdmissionRequest) (*operations.Result, error) {
	var patches []operations.PatchOperation

	state := k8s.LoadZarfState()

	// Default to the InCluster gitURL but check if we initialized with an external server
	gitServerURL := config.ZarfInClusterGitServiceURL
	if !state.GitServerInfo.InternalServer {
		gitServerURL = state.GitServerInfo.GitAddress

		if state.GitServerInfo.GitPort != 0 {
			gitServerURL += fmt.Sprintf(":%d", state.GitServerInfo.GitPort)
		}
	}

	// parse to simple struct to read the git url
	gitRepo := &GenericGitRepo{}
	if err := json.Unmarshal(r.Object.Raw, &gitRepo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %v", err)
	}

	message.Info(gitRepo.Spec.URL)

	replacedURL := git.MutateGitUrlsInText(gitServerURL, gitRepo.Spec.URL)
	patches = append(patches, operations.ReplacePatchOperation("/spec/url", replacedURL))

	// If a prior secret exists, replace it
	if gitRepo.Spec.SecretRef.Name != "" {
		patches = append(patches, operations.ReplacePatchOperation("/spec/secretRef/name", config.ZarfGitServerSecretName))
	} else {
		// Otherwise, add the new secret
		patches = append(patches, operations.AddPatchOperation("/spec/secretRef", SecretRef{Name: config.ZarfGitServerSecretName}))
	}

	return &operations.Result{
		Allowed:  true,
		PatchOps: patches,
	}, nil
}
