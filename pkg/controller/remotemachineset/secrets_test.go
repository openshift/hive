package remotemachineset

import "testing"

func TestWorkerUserData(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput string
		shouldErr      bool
	}{
		{
			name:           "pre 4.6",
			input:          "4.4.0",
			expectedOutput: "worker-user-data",
		},
		{
			name:           "post 4.6",
			input:          "4.6.0",
			expectedOutput: "worker-user-data-managed",
		},
		{
			name:      "error",
			input:     "unparseable",
			shouldErr: true,
		},
	}

	for _, test := range tests {
		output, err := workerUserData(test.input)

		if (err != nil) != test.shouldErr {
			t.Errorf("test %s failure state should have been %t but was not", test.name, test.shouldErr)
		}

		if output != test.expectedOutput {
			t.Errorf("test %s failed because output %s did not match expected output %s", test.name, output, test.expectedOutput)
		}
	}
}
