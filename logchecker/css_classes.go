package logchecker

// CSS annotation class constants used for HTML output.
// These correspond to the styles defined in the frontend viewer.
// See AGENTS.md §3 for the complete class list and meanings.
const (
	cssGood    = "good"    // green — expected/correct value
	cssBad     = "bad"     // red — definite problem
	cssBadish  = "badish"  // yellow — suspicious / minor issue
	cssGoodish = "goodish" // cyan — acceptable but not ideal
	cssLog1    = "log1"    // underline — version/date info
	cssLog3    = "log3"    // blue — file / technical data
	cssLog4    = "log4"    // bold — field values
	cssLog5    = "log5"    // underline — field labels
)
