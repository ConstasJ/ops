package config

import (
	"strings"
	"time"

	"ops-go/pkg/domain"
)

const (
	LabelPrefix        = "ops."
	LabelGuard         = "ops.guard"
	LabelKind          = "ops.kind"
	LabelProbeHTTP     = "ops.probe.http"
	LabelProbeTCP      = "ops.probe.tcp"
	LabelProbeInsecure = "ops.probe.insecure"
	LabelTimeout       = "ops.timeout"
	LabelSettle        = "ops.settle"
	LabelNotes         = "ops.notes"
	LabelExtraDump     = "ops.extra_dump"
)

// ParseLabels 从 Docker 容器的 Label 字典中解析声明式 ContainerSpec
// 若字典中未包含任何以 "ops." 开头的标签，返回 nil
func ParseLabels(labels map[string]string) *domain.ContainerSpec {
	if len(labels) == 0 {
		return nil
	}

	hasOpsLabel := false
	for k := range labels {
		if strings.HasPrefix(k, LabelPrefix) {
			hasOpsLabel = true
			break
		}
	}
	if !hasOpsLabel {
		return nil
	}

	spec := &domain.ContainerSpec{}

	if val, ok := labels[LabelGuard]; ok {
		spec.Guard = domain.GuardLevel(strings.TrimSpace(val))
	}

	if val, ok := labels[LabelKind]; ok {
		spec.Kind = strings.TrimSpace(val)
	}

	if val, ok := labels[LabelNotes]; ok {
		spec.Notes = strings.TrimSpace(val)
	}

	if val, ok := labels[LabelExtraDump]; ok {
		spec.ExtraDump = strings.TrimSpace(val)
	}

	if val, ok := labels[LabelTimeout]; ok {
		if d, err := time.ParseDuration(strings.TrimSpace(val)); err == nil {
			spec.Timeout = int(d.Seconds())
		}
	}

	if val, ok := labels[LabelSettle]; ok {
		if d, err := time.ParseDuration(strings.TrimSpace(val)); err == nil {
			spec.Settle = int(d.Seconds())
		}
	}

	// 探针配置
	httpProbe := strings.TrimSpace(labels[LabelProbeHTTP])
	tcpProbe := strings.TrimSpace(labels[LabelProbeTCP])
	insecureProbe := strings.EqualFold(strings.TrimSpace(labels[LabelProbeInsecure]), "true")

	if httpProbe != "" || tcpProbe != "" || insecureProbe {
		spec.Probe = &domain.ProbeSpec{
			HTTP:     httpProbe,
			TCP:      tcpProbe,
			Insecure: insecureProbe,
		}
	}

	return spec
}

// MergeSpec 合并基础配置（如来自 containers.json）与从 Label 声明中提取的配置，
// 若 override 中指定了非零值字段，优先采用 override
func MergeSpec(base domain.ContainerSpec, override *domain.ContainerSpec) domain.ContainerSpec {
	if override == nil {
		return base
	}

	res := base

	if override.Guard != "" {
		res.Guard = override.Guard
	}
	if override.Kind != "" {
		res.Kind = override.Kind
	}
	if override.Notes != "" {
		res.Notes = override.Notes
	}
	if override.ExtraDump != "" {
		res.ExtraDump = override.ExtraDump
	}
	if override.Timeout > 0 {
		res.Timeout = override.Timeout
	}
	if override.Settle > 0 {
		res.Settle = override.Settle
	}

	if override.Probe != nil {
		if res.Probe == nil {
			res.Probe = &domain.ProbeSpec{}
		}
		if override.Probe.HTTP != "" {
			res.Probe.HTTP = override.Probe.HTTP
		}
		if override.Probe.TCP != "" {
			res.Probe.TCP = override.Probe.TCP
		}
		if override.Probe.Insecure {
			res.Probe.Insecure = true
		}
	}

	return res
}
