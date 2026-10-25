package atomos

import "testing"

func TestRunOptionConfigAndWorkingPathLastWins(t *testing.T) {
	tests := []struct {
		name            string
		opts            []RunOption
		wantConfig      bool
		wantWorkingPath bool
	}{
		{
			name: "config after working path",
			opts: []RunOption{
				WithWorkingPath("/work", "cosmos", "node", LogLevel_Debug, nil),
				WithConfigFile("final.yaml"),
			},
			wantConfig: true,
		},
		{
			name: "working path after config",
			opts: []RunOption{
				WithConfigFile("first.yaml"),
				WithWorkingPath("/work", "cosmos", "node", LogLevel_Debug, nil),
			},
			wantWorkingPath: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config runConfig
			for _, opt := range tt.opts {
				opt(&config)
			}
			if config.hasConfigFile != tt.wantConfig {
				t.Fatalf("hasConfigFile = %v, want %v", config.hasConfigFile, tt.wantConfig)
			}
			if config.useWorkingPath != tt.wantWorkingPath {
				t.Fatalf("useWorkingPath = %v, want %v", config.useWorkingPath, tt.wantWorkingPath)
			}
			if tt.wantConfig && config.configFile != "final.yaml" {
				t.Fatalf("configFile = %q, want final.yaml", config.configFile)
			}
			if tt.wantWorkingPath && config.workingPath != "/work" {
				t.Fatalf("workingPath = %q, want /work", config.workingPath)
			}
		})
	}
}
