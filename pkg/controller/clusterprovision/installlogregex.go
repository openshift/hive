package clusterprovision

// installLogRegex is a struct that represents all the data we use to scan for certain
// search strings in install logs. These structs are serialized as yaml and stored/read from
// the install-log-regexes ConfigMap.
type installLogRegex struct {
	// Name is the name of the regex.
	Name string `json:"name"`

	// SearchRegexStrings are the regex strings we will search for.
	SearchRegexStrings []string `json:"searchRegexStrings"`

	// InstallFailingReason is the single word CamelCase reason we report for this failure in conditions, metrics and logs.
	InstallFailingReason string `json:"installFailingReason"`

	// InstallFailingMessage is the user friendly sentence we report for this failure and conditions, metrics and logs.
	InstallFailingMessage string `json:"installFailingMessage"`
}
