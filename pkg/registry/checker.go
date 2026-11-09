package registry

import (
	"context"
	"fmt"
)

// CheckUpdate 查询远端仓库并评估指定镜像是否具有可用更新
func CheckUpdate(ctx context.Context, client Client, imageRef string, localDigest string) (UpdateCandidate, error) {
	if client == nil {
		return UpdateCandidate{}, fmt.Errorf("registry client is nil")
	}

	tags, err := client.ListTags(ctx, imageRef)
	if err != nil {
		return UpdateCandidate{}, fmt.Errorf("查询 Tags 失败: %w", err)
	}

	// 尝试获取当前镜像 tag 在远端的摘要（若失败降级为空，不阻断 SemVer 判定）
	remoteDigest, _ := client.GetDigest(ctx, imageRef)

	cand := EvaluateUpdate(imageRef, tags, localDigest, remoteDigest)
	return cand, nil
}
