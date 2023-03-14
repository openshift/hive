package helpers

import (
	"math/rand"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	shortSuffix = "deploy"
	// hashLen is 10 to represent "-" + hash + "-" (hash itself is always 8 characters)
	hashLen = 10
)

func TestGetName(t *testing.T) {
	for i := 0; i < 10; i++ {
		// shortBase + "-" + hash will be underneath the maximum limit without any truncation
		shortBase := randSeq(rand.Intn(validation.DNS1123SubdomainMaxLength-hashLen) + 1)

		// Any usage of longBase/longSuffix will definitely need to be truncated
		longBase := randSeq(validation.DNS1123SubdomainMaxLength + rand.Intn(100) + 1)
		longSuffix := randSeq(validation.DNS1123SubdomainMaxLength + rand.Intn(100) + 1)

		tests := []struct {
			name, base, suffix, expected string
		}{
			{
				name:     "Short base and short suffix without truncation",
				base:     shortBase,
				suffix:   shortSuffix,
				expected: shortBase + "-deploy",
			},
			{
				name:     "Long base and short suffix truncates base and inserts hash",
				base:     longBase,
				suffix:   shortSuffix,
				expected: longBase[:validation.DNS1123SubdomainMaxLength-hashLen-len(shortSuffix)] + "-" + hash(longBase) + "-" + shortSuffix,
			},
			{
				name:     "Short base and long suffix inserts a hash instead of appending suffix",
				base:     shortBase,
				suffix:   longSuffix,
				expected: shortBase + "-" + hash(shortBase+"-"+longSuffix),
			},
			{
				name:     "Empty base and short suffix",
				base:     "",
				suffix:   shortSuffix,
				expected: "-" + shortSuffix,
			},
			{
				name:     "Empty base and long suffix appends hash of suffix",
				base:     "",
				suffix:   longSuffix,
				expected: "-" + hash("-"+longSuffix),
			},
			{
				name:     "Short base and no suffix",
				base:     shortBase,
				suffix:   "",
				expected: shortBase + "-",
			},
			{
				name:     "Long base and no suffix",
				base:     longBase,
				suffix:   "",
				expected: longBase[:validation.DNS1123SubdomainMaxLength-hashLen] + "-" + hash(longBase) + "-",
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				result := GetName(test.base, test.suffix, validation.DNS1123SubdomainMaxLength)
				if result != test.expected {
					t.Errorf("Got unexpected result. \nExpected: %s\nGot:      %s", test.expected, result)
				}
			})
		}
	}
}

func TestGetNameIsDifferent(t *testing.T) {
	shortName := randSeq(32)
	deployerName := GetName(shortName, "deploy", validation.DNS1123SubdomainMaxLength)
	builderName := GetName(shortName, "build", validation.DNS1123SubdomainMaxLength)
	if deployerName == builderName {
		t.Errorf("Expecting names to be different: %s\n", deployerName)
	}
	longName := randSeq(validation.DNS1123SubdomainMaxLength + 10)
	deployerName = GetName(longName, "deploy", validation.DNS1123SubdomainMaxLength)
	builderName = GetName(longName, "build", validation.DNS1123SubdomainMaxLength)
	if deployerName == builderName {
		t.Errorf("Expecting names to be different: %s\n", deployerName)
	}
}

func TestGetNameReturnShortNames(t *testing.T) {
	base := randSeq(32)
	for maxLength := 0; maxLength < len(base)+2; maxLength++ {
		for suffixLen := 0; suffixLen <= maxLength+1; suffixLen++ {
			suffix := randSeq(suffixLen)
			got := GetName(base, suffix, maxLength)
			if len(got) > maxLength {
				t.Fatalf("len(GetName(%[1]q, %[2]q, %[3]d)) = len(%[4]q) = %[5]d; want %[3]d", base, suffix, maxLength, got, len(got))
			}
		}
	}
}

func randSeq(n int) string {
	// From k8s.io/kubernetes/pkg/api/generator.go
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789-")

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
