//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 守护：在线更新/回滚必须从我们的仓库取发布（带首字加速），而不是官方仓库。
// 官方若改了仓库名的来源（改常量、换配置），这里会失败，提醒把改动重新套上。
const wantReleaseRepo = "234150476/sub2api"

type repoRecordingReleaseClient struct {
	updateServiceGitHubClientStub
	repos []string
}

func (c *repoRecordingReleaseClient) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	c.repos = append(c.repos, repo)
	return &GitHubRelease{TagName: "v0.0.1", Name: "v0.0.1"}, nil
}

func (c *repoRecordingReleaseClient) FetchRecentReleases(_ context.Context, repo string, _ int) ([]*GitHubRelease, error) {
	c.repos = append(c.repos, repo)
	return nil, nil
}

func TestUpdateServiceUsesOurReleaseRepo(t *testing.T) {
	client := &repoRecordingReleaseClient{}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.0.1", "release")

	_, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	_, err = svc.ListRollbackVersions(context.Background())
	require.NoError(t, err)

	require.Len(t, client.repos, 2, "check-update and rollback list should each query releases once")
	for _, repo := range client.repos {
		require.Equal(t, wantReleaseRepo, repo)
	}
}
