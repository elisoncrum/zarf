package git

import (
	"fmt"
	"path/filepath"

	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/internal/k8s"
	"github.com/defenseunicorns/zarf/src/internal/message"
	"github.com/defenseunicorns/zarf/src/internal/utils"
	"github.com/go-git/go-git/v5"
	goConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

const offlineRemoteName = "offline-downstream"
const onlineRemoteRefPrefix = "refs/remotes/" + onlineRemoteName + "/"

func PushAllDirectories(localPath string) error {
	gitServerInfo := config.GetGitServerInfo()
	gitServerURL := ""

	if gitServerInfo.InternalServer {
		// This is an internal gitea server so we'll use what we normally do..
		// Establish a git tunnel to the internal gitea pod to send the repos
		tunnel := k8s.NewZarfTunnel()
		tunnel.Connect(k8s.ZarfGit, false)
		defer tunnel.Close()

		gitServerURL = fmt.Sprintf("http://%s", tunnel.Endpoint())
	} else {
		// This is an external git server so we'll use whatever we were provided with
		gitServerURL = gitServerInfo.GitAddress

		// Add a port to the URL if it was provided
		if gitServerInfo.GitPort != 0 {
			gitServerURL = fmt.Sprintf("%s:%d", gitServerInfo.GitAddress, gitServerInfo.GitPort)
		}
	}

	paths, err := utils.ListDirectories(localPath)
	if err != nil {
		message.Warnf("Unable to list the %s directory", localPath)
		return err
	}

	spinner := message.NewProgressSpinner("Processing %d git repos", len(paths))
	defer spinner.Stop()

	for _, path := range paths {
		basename := filepath.Base(path)
		spinner.Updatef("Pushing git repo %s", basename)

		repo, err := prepRepoForPush(path, gitServerURL, gitServerInfo.GitUsername)
		if err != nil {
			message.Warnf("error when preping the repo for push.. %v", err)
			return err
		}

		if err := push(repo, path, spinner); err != nil {
			spinner.Warnf("Unable to push the git repo %s", basename)
			return err
		}

		// // Add the read-only user to this repo
		// repoPathSplit := strings.Split(path, "/")
		// repoNameWithGitTag := repoPathSplit[len(repoPathSplit)-1]
		// repoName := strings.Split(repoNameWithGitTag, ".git")[0]
		// err = addReadOnlyUserToRepo(gitServerURL, repoName)
		// if err != nil {
		// 	message.Warnf("Unable to add the read-only user to the repo: %s\n", repoName)
		// 	return err
		// }
	}

	spinner.Success()
	return nil
}

func prepRepoForPush(localPath, tunnelUrl, username string) (*git.Repository, error) {
	// Open the given repo
	repo, err := git.PlainOpen(localPath)
	if err != nil {
		return nil, fmt.Errorf("not a valid git repo or unable to open: %w", err)
	}

	// Get the upstream URL
	remote, err := repo.Remote(onlineRemoteName)
	if err != nil {
		return nil, fmt.Errorf("unable to find the git remote: %w", err)
	}

	remoteUrl := remote.Config().URLs[0]
	targetUrl := transformURL(tunnelUrl, remoteUrl, username)

	_, err = repo.CreateRemote(&goConfig.RemoteConfig{
		Name: offlineRemoteName,
		URLs: []string{targetUrl},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create offline remote: %w", err)
	}

	return repo, nil
}

func push(repo *git.Repository, localPath string, spinner *message.Spinner) error {

	gitCred := http.BasicAuth{
		// Username: "gitea",
		// Password: "password",
		Username: config.ZarfGitPushUser,
		Password: config.GetSecret(config.StateGitPush),
	}

	// Since we are pushing HEAD:refs/heads/master on deployment, leaving
	// duplicates of the HEAD ref (ex. refs/heads/master,
	// refs/remotes/online-upstream/master, will cause the push to fail)
	removedRefs, err := removeHeadCopies(localPath)
	if err != nil {
		return fmt.Errorf("unable to remove unused git refs from the repo: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		RemoteName: offlineRemoteName,
		Auth:       &gitCred,
		Progress:   spinner,
		// If a provided refspec doesn't push anything, it is just ignored
		RefSpecs: []goConfig.RefSpec{
			"refs/heads/*:refs/heads/*",
			onlineRemoteRefPrefix + "*:refs/heads/*",
			"refs/tags/*:refs/tags/*",
		},
	})
	if err == git.NoErrAlreadyUpToDate {
		spinner.Debugf("Repo already up-to-date")
	} else if err != nil {
		return fmt.Errorf("unable to push repo to the gitops service: %w", err)
	}

	// Add back the refs we removed just incase this push isn't the last thing
	// being run and a later task needs to reference them.
	addRefs(localPath, removedRefs)

	return nil
}
