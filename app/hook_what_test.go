package main

import "testing"

// What a command runs, for the line "what the agent does now": the program and its subcommand, never the arguments.
func TestTheProgramOfACommand(t *testing.T) {
	for cmd, want := range map[string]string{
		"go test ./app -run TestX":                        "go test",
		"FOO=1 npm run build -- --token=secret":           "npm run",
		"cd /srv/app && docker compose up -d":             "docker compose",
		"/usr/local/bin/php artisan migrate":              "php artisan",
		"curl -s -H 'Authorization: Bearer abc' http://x": "curl",
		"ls":                 "ls",
		"":                   "",
		"$(cat secret) | sh": "",
	} {
		eq(t, programOf(cmd), want, cmd)
	}
}
