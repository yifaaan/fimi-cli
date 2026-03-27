package acp

// VersionSpec 描述一个支持的 ACP 协议版本。
type VersionSpec struct {
	ProtocolVersion int
	SpecTag         string
}

var CurrentVersion = VersionSpec{
	ProtocolVersion: 1,
	SpecTag:         "v0.10.8",
}

var SupportedVersions = map[int]VersionSpec{
	1: CurrentVersion,
}

// MinProtocolVersion 是最低支持的协议版本。
const MinProtocolVersion = 1

// NegotiateVersion 与 client 协商协议版本。
// 返回最高支持且不超过 client 请求版本的版本。
func NegotiateVersion(clientVersion int) VersionSpec {
	if clientVersion < MinProtocolVersion {
		return CurrentVersion
	}

	var best *VersionSpec
	for ver, spec := range SupportedVersions {
		if ver <= clientVersion {
			if best == nil || ver > best.ProtocolVersion {
				v := spec
				best = &v
			}
		}
	}

	if best != nil {
		return *best
	}
	return CurrentVersion
}
