package registry

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// UpdateType 升级类型
type UpdateType string

const (
	UpdateNone    UpdateType = "none"
	UpdateDigest  UpdateType = "digest"
	UpdatePatch   UpdateType = "patch"
	UpdateMinor   UpdateType = "minor"
	UpdateMajor   UpdateType = "major"
	UpdateCompose UpdateType = "compose"
)

// UpdateCandidate 镜像升级评估结果
type UpdateCandidate struct {
	HasUpdate     bool       `json:"has_update"`
	CurrentTag    string     `json:"current_tag"`
	TargetTag     string     `json:"target_tag"`
	UpdateType    UpdateType `json:"update_type"`
	CurrentDigest string     `json:"current_digest,omitempty"`
	RemoteDigest  string     `json:"remote_digest,omitempty"`
	Reason        string     `json:"reason"`
}

var (
	// 匹配预发布版本关键词
	prereleaseRegex = regexp.MustCompile(`(?i)(?:-|\.)(alpha|beta|rc|dev|preview|snapshot|test|testing|canary)`)
	// 提取版本号和发行版变体（例如 1.25.4-alpine3.19 -> sem: 1.25.4, flavor: -alpine3.19）
	semverFlavorRegex = regexp.MustCompile(`^v?(\d+(?:\.\d+)*(?:-[a-zA-Z0-9.]+)?)(.*)$`)
)

type tagVersionInfo struct {
	tag     string
	version *semver.Version
	flavor  string
}

// splitTag 分解 tag 为版本号部分与 flavor 后缀
func splitTag(tag string) (baseVer string, flavor string) {
	// 如果包含 -，查找第一个 - 后面的非数字部分
	idx := strings.Index(tag, "-")
	if idx == -1 {
		return tag, ""
	}
	// 如果包含 -，检查是否属于预发布版本，否则将第一个 - 作为 flavor 分界
	if prereleaseRegex.MatchString(tag) {
		return tag, ""
	}
	return tag[:idx], tag[idx:]
}

func parseTag(tag string) *tagVersionInfo {
	base, flavor := splitTag(tag)
	v, err := semver.NewVersion(base)
	if err != nil {
		// 再尝试直接解析原 tag
		v, err = semver.NewVersion(tag)
		if err != nil {
			return nil
		}
		flavor = ""
	}
	return &tagVersionInfo{
		tag:     tag,
		version: v,
		flavor:  flavor,
	}
}

// EvaluateUpdate 根据当前镜像引用与远端 Tags/Digest 列表评估是否存在更新建议
func EvaluateUpdate(currentRef string, remoteTags []string, currentDigest, remoteDigest string) UpdateCandidate {
	_, currentTag, err := ParseImageReference(currentRef)
	if err != nil {
		// 容错：如果解析失败，截取 : 后的字符串
		idx := strings.LastIndex(currentRef, ":")
		if idx != -1 {
			currentTag = currentRef[idx+1:]
		} else {
			currentTag = "latest"
		}
	}

	result := UpdateCandidate{
		HasUpdate:     false,
		CurrentTag:    currentTag,
		TargetTag:     currentTag,
		UpdateType:    UpdateNone,
		CurrentDigest: currentDigest,
		RemoteDigest:  remoteDigest,
		Reason:        "当前已是最新版本",
	}

	// 1. 如果当前 tag 是 latest 或无法解析为语义版本的静态 tag
	currentInfo := parseTag(currentTag)
	if currentInfo == nil {
		// 纯 Digest 比对
		if currentDigest != "" && remoteDigest != "" && currentDigest != remoteDigest {
			result.HasUpdate = true
			result.UpdateType = UpdateDigest
			result.Reason = "远端 Manifest 摘要已更新 (Digest float)"
		}
		return result
	}

	isCurrentPrerelease := currentInfo.version.Prerelease() != "" || prereleaseRegex.MatchString(currentTag)

	// 2. 遍历远端 tags，寻找兼容 flavor 的更高版本
	var bestCandidate *tagVersionInfo
	for _, rTag := range remoteTags {
		if rTag == currentTag {
			continue
		}

		// 默认过滤非稳定版
		if !isCurrentPrerelease && (prereleaseRegex.MatchString(rTag)) {
			continue
		}

		info := parseTag(rTag)
		if info == nil {
			continue
		}

		// 如果当前 tag 有特定的 flavor（例如 -alpine），候选 tag 必须保留相同 flavor
		if currentInfo.flavor != "" && info.flavor != currentInfo.flavor {
			continue
		}
		// 如果当前 tag 没有 flavor，候选 tag 也不应带有如 -alpine/-bookworm 等 flavor
		if currentInfo.flavor == "" && info.flavor != "" {
			continue
		}

		// 必须大于当前版本
		if !info.version.GreaterThan(currentInfo.version) {
			continue
		}

		// 策略：优先选择同 major 下的最高版本，避免破坏性大版本升级
		if bestCandidate == nil {
			bestCandidate = info
			continue
		}

		// 如果当前候选是同 major，而新遍历到的也是同 major，比谁更高
		if info.version.Major() == currentInfo.version.Major() {
			if bestCandidate.version.Major() != currentInfo.version.Major() || info.version.GreaterThan(bestCandidate.version) {
				bestCandidate = info
			}
		} else if bestCandidate.version.Major() != currentInfo.version.Major() {
			// 两者都是不同 major，选版本更高的那个
			if info.version.GreaterThan(bestCandidate.version) {
				bestCandidate = info
			}
		}
	}

	if bestCandidate != nil {
		result.HasUpdate = true
		result.TargetTag = bestCandidate.tag

		if bestCandidate.version.Major() > currentInfo.version.Major() {
			result.UpdateType = UpdateMajor
			result.Reason = "发现新大版本 (Major)"
		} else if bestCandidate.version.Minor() > currentInfo.version.Minor() {
			result.UpdateType = UpdateMinor
			result.Reason = "发现新次版本 (Minor)"
		} else {
			result.UpdateType = UpdatePatch
			result.Reason = "发现补丁更新 (Patch)"
		}
		return result
	}

	// 3. 如果 SemVer 没有更高版本，但当前版本在远端存在且 Digest 变动（例如 alpine 补丁刷新）
	if currentDigest != "" && remoteDigest != "" && currentDigest != remoteDigest {
		result.HasUpdate = true
		result.UpdateType = UpdateDigest
		result.Reason = "远端镜像层摘要已刷新 (Digest float)"
		return result
	}

	return result
}

// CompareImageRefs 比较两个镜像引用的版本差异类型（major / minor / patch / digest / compose）
func CompareImageRefs(currentRef, targetRef string) UpdateCandidate {
	_, currTag, _ := ParseImageReference(currentRef)
	_, targetTag, _ := ParseImageReference(targetRef)
	if currTag == "" {
		currTag = "latest"
	}
	if targetTag == "" {
		targetTag = "latest"
	}

	currInfo := parseTag(currTag)
	targetInfo := parseTag(targetTag)

	updateType := UpdateCompose
	reason := "Compose 声明已变更"

	if currInfo != nil && targetInfo != nil {
		if targetInfo.version.GreaterThan(currInfo.version) {
			if targetInfo.version.Major() > currInfo.version.Major() {
				updateType = UpdateMajor
				reason = "Compose 声明重大版本更新 (Major)"
			} else if targetInfo.version.Minor() > currInfo.version.Minor() {
				updateType = UpdateMinor
				reason = "Compose 声明次版本更新 (Minor)"
			} else {
				updateType = UpdatePatch
				reason = "Compose 声明补丁更新 (Patch)"
			}
		} else if targetInfo.version.LessThan(currInfo.version) {
			reason = "Compose 声明版本降级或回退"
		}
	}

	return UpdateCandidate{
		HasUpdate:  true,
		CurrentTag: currTag,
		TargetTag:  targetTag,
		UpdateType: updateType,
		Reason:     reason,
	}
}

