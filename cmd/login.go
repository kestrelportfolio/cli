package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kestrelportfolio/kestrel-cli/internal/api"
	"github.com/kestrelportfolio/kestrel-cli/internal/config"
	"github.com/spf13/cobra"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the Kestrel Portfolio API",
	Long:  `Saves your API token to ~/.config/kestrel/config.json and validates it against the API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token := loginToken

		// If no --token flag, prompt for it
		if token == "" {
			fmt.Print("API token: ")
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading token: %w", err)
			}
			token = strings.TrimSpace(input)
		}

		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}

		// Use the token to test against /me
		testClient := &api.Client{
			BaseURL:    cfg.BaseURL,
			Token:      token,
			HTTPClient: client.HTTPClient,
		}

		envelope, err := testClient.Get("/me", nil)
		if err != nil {
			return fmt.Errorf("validating token: %w", err)
		}

		// Save the validated token
		updates := &config.Config{
			Token:   token,
			BaseURL: cfg.BaseURL,
		}
		if err := config.SaveGlobal(updates); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		// Show who we authenticated as
		if printer.IsJSON() {
			printer.JSON(envelope.Data)
			return nil
		}

		// Parse the /me response to show a friendly message
		var me struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
			Organization struct {
				Name string `json:"name"`
			} `json:"organization"`
		}
		if err := json.Unmarshal(envelope.Data, &me); err != nil {
			printer.Success("Logged in successfully. Token saved.")
			return nil
		}

		printer.Success(fmt.Sprintf("Logged in as %s (%s)", me.User.Email, me.Organization.Name))
		configPath, _ := config.GlobalConfigPath()
		fmt.Fprintf(os.Stderr, "  Token saved to %s\n", configPath)

		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token (or enter interactively)")
	rootCmd.AddCommand(loginCmd)
}
