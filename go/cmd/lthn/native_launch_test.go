// SPDX-License-Identifier: EUPL-1.2

//go:build !ios && !android

package main

import core "dappco.re/go"

func TestNativeLaunchArgument_GoodAcceptsLetheanURLAndDocument(t *core.T) {
	for _, argument := range []string{
		"lthn://chat",
		"/tmp/profile.lthn",
		`C:\Users\person\profile.LTHN`,
	} {
		core.AssertTrue(t, isNativeLaunchArgument(argument), argument)
	}
}

func TestNativeLaunchArgument_BadRejectsCommandsAndOtherFiles(t *core.T) {
	for _, argument := range []string{
		"serve",
		"https://example.test",
		"/tmp/profile.json",
		"",
	} {
		core.AssertFalse(t, isNativeLaunchArgument(argument), argument)
	}
}

func TestNativeLaunchArgument_UglyRejectsOversizedAndControlInput(t *core.T) {
	core.AssertFalse(t, isNativeLaunchArgument("lthn://"+core.Repeat("x", 4097)))
	core.AssertFalse(t, isNativeLaunchArgument("lthn://chat\nserve"))
}
