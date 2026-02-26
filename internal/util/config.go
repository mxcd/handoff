package util

import "github.com/mxcd/go-config/config"

func InitConfig() error {
	err := config.LoadConfig([]config.Value{
		// version info
		config.String("DEPLOYMENT_IMAGE_TAG").NotEmpty().Default("development"),

		// logging config
		config.String("LOG_LEVEL").NotEmpty().Default("info"),

		// server config
		config.Bool("DEV").Default(false),
		config.Int("PORT").Default(8080),

		// API key auth (required — server refuses to start without at least one key)
		config.StringArray("API_KEYS").NotEmpty(),

		// session and result TTLs
		config.String("SESSION_TTL").Default("30m"),
		config.String("RESULT_TTL").Default("5m"),

		// base URL for generating session URLs (required)
		config.String("BASE_URL").NotEmpty(),
	})
	return err
}
