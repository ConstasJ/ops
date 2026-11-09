package registry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Client 封装与容器镜像仓库的交互接口
type Client interface {
	ListTags(ctx context.Context, imageRef string) ([]string, error)
	GetDigest(ctx context.Context, imageRef string) (string, error)
}

// Option 客户端配置函数
type Option func(*RegistryClient)

// RegistryClient 实现 Client 接口
type RegistryClient struct {
	auth      authn.Authenticator
	keychain  authn.Keychain
	insecure  bool
	transport http.RoundTripper
}

// WithAuth 设置单次显式认证凭据
func WithAuth(auth authn.Authenticator) Option {
	return func(c *RegistryClient) {
		c.auth = auth
	}
}

// WithKeychain 设置凭据管理器，默认为 authn.DefaultKeychain
func WithKeychain(kc authn.Keychain) Option {
	return func(c *RegistryClient) {
		c.keychain = kc
	}
}

// WithInsecure 设置是否允许非 TLS (HTTP) 访问，常用于单元测试或本地私有 Registry
func WithInsecure(insecure bool) Option {
	return func(c *RegistryClient) {
		c.insecure = insecure
	}
}

// WithTransport 设置自定义传输层
func WithTransport(t http.RoundTripper) Option {
	return func(c *RegistryClient) {
		c.transport = t
	}
}

// NewClient 创建 Registry 客户端实例
func NewClient(opts ...Option) *RegistryClient {
	c := &RegistryClient{
		keychain: authn.DefaultKeychain,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ParseImageReference 解析镜像引用，返回规范化的 repo 路径与 tag
func ParseImageReference(imageRef string) (repoName, tagName string, err error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", "", fmt.Errorf("解析镜像引用 %q 失败: %w", imageRef, err)
	}
	repo := ref.Context().Name()
	tag := ref.Identifier()
	return repo, tag, nil
}

func (c *RegistryClient) getRemoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{
		remote.WithContext(ctx),
	}
	if c.auth != nil {
		opts = append(opts, remote.WithAuth(c.auth))
	} else if c.keychain != nil {
		opts = append(opts, remote.WithAuthFromKeychain(c.keychain))
	}
	if c.transport != nil {
		opts = append(opts, remote.WithTransport(c.transport))
	}
	return opts
}

func (c *RegistryClient) parseNameOptions() []name.Option {
	var opts []name.Option
	if c.insecure {
		opts = append(opts, name.Insecure)
	}
	return opts
}

// ListTags 列出指定镜像仓库下的所有 Tags
func (c *RegistryClient) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	nameOpts := c.parseNameOptions()
	// 先尝试按完整 reference 解析上下文 repository
	ref, err := name.ParseReference(imageRef, nameOpts...)
	var repo name.Repository
	if err == nil {
		repo = ref.Context()
	} else {
		// 如果不带 tag（例如只有 library/postgres），尝试按 repository 解析
		repo, err = name.NewRepository(imageRef, nameOpts...)
		if err != nil {
			return nil, fmt.Errorf("解析仓库引用 %q 失败: %w", imageRef, err)
		}
	}

	remoteOpts := c.getRemoteOptions(ctx)
	tags, err := remote.List(repo, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("获取仓库 %s 的 Tags 列表失败: %w", repo.Name(), err)
	}
	return tags, nil
}

// GetDigest 获取指定镜像引用的远端 Manifest Digest (sha256:xxxx)
func (c *RegistryClient) GetDigest(ctx context.Context, imageRef string) (string, error) {
	nameOpts := c.parseNameOptions()
	ref, err := name.ParseReference(imageRef, nameOpts...)
	if err != nil {
		return "", fmt.Errorf("解析镜像引用 %q 失败: %w", imageRef, err)
	}

	remoteOpts := c.getRemoteOptions(ctx)

	// 优先使用 Head 获取精简 Descriptor
	desc, err := remote.Head(ref, remoteOpts...)
	if err == nil && desc.Digest.String() != "" {
		return desc.Digest.String(), nil
	}

	// 部分 Registry 不支持或对 HEAD 返回 404/405 时，降级调用 Get
	img, err := remote.Get(ref, remoteOpts...)
	if err != nil {
		return "", fmt.Errorf("获取镜像 %s 的 Digest 失败: %w", imageRef, err)
	}
	return img.Digest.String(), nil
}
