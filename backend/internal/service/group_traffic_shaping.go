package service

// GroupTrafficShaping 描述某个分组的流量伪装策略快照。
//
// 这个结构体是热路径不可变值对象——一次请求里只读不写。每个开关都映射到
// service.Group 上同名字段；总开关 Enabled=false 时所有子开关都视为 false。
//
// 用途：sub2api 提供"对外友好型分组"能力——开启后该分组的请求会进行
// 一些表层处理（极速 SSE 首字节、流式平滑、5xx 静默重试），让第三方
// 聚合站（如 axonhub）感知到的 TTFT 更低、TPS 更稳、成功率更高，
// 同时不影响其他分组。
type GroupTrafficShaping struct {
	Enabled              bool
	SSEEarlyKeepalive    bool
	StreamSmoothing      bool
	SilentRetry5xx       bool
}

// IsZero 判断是否完全未启用任何伪装策略。
func (g GroupTrafficShaping) IsZero() bool {
	return !g.Enabled
}

// resolveGroupTrafficShaping 从已加载的 service.Group 提取流量伪装策略。
//
// 设计要点：
//   - sub2api 网关热路径已经通过 api_key_auth_cache 把 group 加载到内存，
//     所以这里只是字段映射，不再二次查 DB；
//   - 总开关 TrafficShapingEnabled=false 时所有子开关都被强制视为 false，
//     消除误配置风险；
//   - g 为 nil 时返回零值（保持现有默认行为）。
func resolveGroupTrafficShaping(g *Group) GroupTrafficShaping {
	if g == nil || !g.TrafficShapingEnabled {
		return GroupTrafficShaping{}
	}
	return GroupTrafficShaping{
		Enabled:           true,
		SSEEarlyKeepalive: g.SSEEarlyKeepaliveEnabled,
		StreamSmoothing:   g.StreamSmoothingEnabled,
		SilentRetry5xx:    g.SilentRetry5xxEnabled,
	}
}
