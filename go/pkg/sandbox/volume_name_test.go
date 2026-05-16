// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1431 — IsValidVolumeName gate. The attack vector
// is a manifest VolumeMount declaring `Persist: "/var/run/docker.sock"`
// which would otherwise produce `docker run -v /var/run/docker.sock:/X`,
// a host-path bind-mount that root-escapes via the docker socket.
// These tests cover the rejection paths + a few legitimate names.

package sandbox

import core "dappco.re/go"

func TestSandbox_IsValidVolumeName_Good_Simple(t *core.T) {
	core.AssertTrue(t, IsValidVolumeName("data"))
	core.AssertTrue(t, IsValidVolumeName("pg-data"))
	core.AssertTrue(t, IsValidVolumeName("forgejo_state"))
	core.AssertTrue(t, IsValidVolumeName("vault.warden"))
	core.AssertTrue(t, IsValidVolumeName("a"))
	core.AssertTrue(t, IsValidVolumeName("z123"))
}

func TestSandbox_IsValidVolumeName_Bad_DockerSocketEscape(t *core.T) {
	// The exact attack pattern Cerberus flagged.
	core.AssertFalse(t, IsValidVolumeName("/var/run/docker.sock"))
}

func TestSandbox_IsValidVolumeName_Bad_LeadingSlash(t *core.T) {
	core.AssertFalse(t, IsValidVolumeName("/etc/passwd"))
	core.AssertFalse(t, IsValidVolumeName("/"))
}

func TestSandbox_IsValidVolumeName_Bad_LeadingDot(t *core.T) {
	// Docker would treat this as a relative-path bind-mount.
	core.AssertFalse(t, IsValidVolumeName(".relative"))
	core.AssertFalse(t, IsValidVolumeName(".."))
	core.AssertFalse(t, IsValidVolumeName("../escape"))
}

func TestSandbox_IsValidVolumeName_Bad_LeadingDash(t *core.T) {
	// Could be parsed as a flag in docker run argument vector.
	core.AssertFalse(t, IsValidVolumeName("-rf"))
	core.AssertFalse(t, IsValidVolumeName("--privileged"))
}

func TestSandbox_IsValidVolumeName_Bad_SlashInMiddle(t *core.T) {
	core.AssertFalse(t, IsValidVolumeName("data/escape"))
	core.AssertFalse(t, IsValidVolumeName("home/user"))
}

func TestSandbox_IsValidVolumeName_Bad_Empty(t *core.T) {
	core.AssertFalse(t, IsValidVolumeName(""))
}

func TestSandbox_IsValidVolumeName_Bad_TooLong(t *core.T) {
	// 65 chars — one over the cap.
	long := "a"
	for i := 0; i < 64; i++ {
		long += "x"
	}
	core.AssertFalse(t, IsValidVolumeName(long))
}

func TestSandbox_IsValidVolumeName_Bad_SpecialChars(t *core.T) {
	core.AssertFalse(t, IsValidVolumeName("data:other"))
	core.AssertFalse(t, IsValidVolumeName("data;rm"))
	core.AssertFalse(t, IsValidVolumeName("data space"))
	core.AssertFalse(t, IsValidVolumeName("data$(pwd)"))
}

// Cerberus Mantis #1446 — container-path gate (child of #1431).
// Same shape primitive as IsValidVolumeName; this catches the
// container-side of the `-v <name>:<container>` arg.

func TestSandbox_IsValidContainerPath_Good(t *core.T) {
	core.AssertTrue(t, IsValidContainerPath("/data"))
	core.AssertTrue(t, IsValidContainerPath("/var/lib/postgresql/data"))
	core.AssertTrue(t, IsValidContainerPath("/srv/app"))
}

func TestSandbox_IsValidContainerPath_Bad_MountOptionInjection(t *core.T) {
	// The exact attack pattern Cerberus flagged.
	core.AssertFalse(t, IsValidContainerPath("/data:ro,bind,private"))
	core.AssertFalse(t, IsValidContainerPath("/data:ro"))
	core.AssertFalse(t, IsValidContainerPath("/data:rw"))
}

func TestSandbox_IsValidContainerPath_Bad_NoLeadingSlash(t *core.T) {
	core.AssertFalse(t, IsValidContainerPath("data"))
	core.AssertFalse(t, IsValidContainerPath("relative/path"))
}

func TestSandbox_IsValidContainerPath_Bad_CommaSeparator(t *core.T) {
	core.AssertFalse(t, IsValidContainerPath("/data,bind"))
}

func TestSandbox_IsValidContainerPath_Bad_Whitespace(t *core.T) {
	core.AssertFalse(t, IsValidContainerPath("/data path"))
	core.AssertFalse(t, IsValidContainerPath("/data\tpath"))
}

func TestSandbox_IsValidContainerPath_Bad_Empty(t *core.T) {
	core.AssertFalse(t, IsValidContainerPath(""))
}

func TestSandbox_IsValidContainerPath_Bad_NullByte(t *core.T) {
	core.AssertFalse(t, IsValidContainerPath("/data\x00escape"))
}

// TestSandbox_buildLongRunArgs_HardenedDefaults — Cerberus Mantis
// #1434 — assert that every `docker run` for a long-running bundle
// container gets the cap-drop + no-new-privileges + pids-limit
// hardening flags. Regression guard: if these get dropped or moved,
// the test fails loudly rather than silently shipping permissive
// containers.
func TestSandbox_buildLongRunArgs_HardenedDefaults(t *core.T) {
	s := &Service{}
	input := SpawnLongInput{
		Image:   "alpine",
		Command: "sleep",
	}
	args := s.buildLongRunArgs("docker", "lthn-test", 0, input)
	var sawCapDrop, sawNoNewPrivs, sawPidsLimit bool
	for _, a := range args {
		if a == "--cap-drop=ALL" {
			sawCapDrop = true
		}
		if a == "--security-opt=no-new-privileges" {
			sawNoNewPrivs = true
		}
		if a == "--pids-limit=512" {
			sawPidsLimit = true
		}
	}
	core.AssertTrue(t, sawCapDrop,
		"docker run must include --cap-drop=ALL (Mantis #1434)")
	core.AssertTrue(t, sawNoNewPrivs,
		"docker run must include --security-opt=no-new-privileges (Mantis #1434)")
	core.AssertTrue(t, sawPidsLimit,
		"docker run must include --pids-limit=512 (Mantis #1434)")
}

// TestSandbox_buildLongRunArgs_RejectsBadVolume — defence-in-depth
// check that buildLongRunArgs silently skips an invalid volume name
// even if marketplace's validator was bypassed (which it shouldn't
// be, but the sandbox is the safety floor — see memory
// design_sandbox_is_the_safety_floor).
func TestSandbox_buildLongRunArgs_RejectsBadVolume(t *core.T) {
	s := &Service{}
	input := SpawnLongInput{
		Image:   "alpine",
		Command: "sleep",
		Volumes: []LongVolumeMount{
			{Name: "/var/run/docker.sock", Container: "/sock"},
			{Name: "good-vol", Container: "/data"},
		},
	}
	args := s.buildLongRunArgs("docker", "lthn-test", 0, input)
	// The good volume should appear; the bad one must not.
	var sawBad, sawGood bool
	for i, a := range args {
		if a == "/var/run/docker.sock:/sock" {
			sawBad = true
		}
		if a == "good-vol:/data" {
			sawGood = true
			// The previous arg should be -v.
			if i > 0 {
				core.AssertEqual(t, "-v", args[i-1])
			}
		}
	}
	core.AssertFalse(t, sawBad,
		"buildLongRunArgs must filter out invalid-name volumes")
	core.AssertTrue(t, sawGood,
		"buildLongRunArgs must keep valid-name volumes")
}
