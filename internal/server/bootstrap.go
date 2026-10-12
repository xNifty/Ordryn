package server

import (
	"errors"
	"fmt"
	"strings"

	"GoTodo/internal/config"
	"GoTodo/internal/storage"

	"golang.org/x/crypto/bcrypt"
)

const bootstrapAPIKeyName = "bootstrap"

// RunBootstrap applies optional first-boot env configuration (idempotent).
//
//	ORDRYN_BOOTSTRAP_ADMIN_EMAIL / ORDRYN_BOOTSTRAP_ADMIN_PASSWORD — create admin if missing
//	ORDRYN_BOOTSTRAP_ENABLE_API=true — set site_settings.enable_api
//	ORDRYN_BOOTSTRAP_CREATE_API_KEY=true — mint a named "bootstrap" API key (once) for that admin
//	ORDRYN_BOOTSTRAP_SITE_MODE=multi|company — set the site mode while no projects or organizations exist
func RunBootstrap() error {
	email := strings.TrimSpace(config.BrandEnv("BOOTSTRAP_ADMIN_EMAIL"))
	password := config.BrandEnv("BOOTSTRAP_ADMIN_PASSWORD")
	enableAPI := strings.EqualFold(strings.TrimSpace(config.BrandEnv("BOOTSTRAP_ENABLE_API")), "true")
	createKey := strings.EqualFold(strings.TrimSpace(config.BrandEnv("BOOTSTRAP_CREATE_API_KEY")), "true")
	siteMode := strings.TrimSpace(config.BrandEnv("BOOTSTRAP_SITE_MODE"))

	if siteMode != "" {
		if err := bootstrapSiteMode(siteMode); err != nil {
			return err
		}
	}

	if email == "" && !enableAPI && !createKey {
		return nil
	}

	var adminID int
	if email != "" {
		if password == "" {
			return fmt.Errorf("ORDRYN_BOOTSTRAP_ADMIN_PASSWORD is required when ORDRYN_BOOTSTRAP_ADMIN_EMAIL is set")
		}
		exists, err := storage.UserExistsByEmail(email)
		if err != nil {
			return fmt.Errorf("bootstrap admin lookup: %w", err)
		}
		if exists {
			adminID, err = storage.GetUserIDByEmail(email)
			if err != nil {
				return fmt.Errorf("bootstrap admin id: %w", err)
			}
			fmt.Printf("Bootstrap: admin user %q already exists (id=%d); leaving as-is\n", email, adminID)
		} else {
			roleID, err := storage.GetRoleIDByName("admin")
			if err != nil {
				return fmt.Errorf("bootstrap admin role: %w", err)
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("bootstrap hash password: %w", err)
			}
			tz := config.Cfg.DefaultTimezone
			if tz == "" {
				tz = "America/New_York"
			}
			adminID, err = storage.CreateUser(email, string(hash), tz, roleID)
			if err != nil {
				return fmt.Errorf("bootstrap create admin: %w", err)
			}
			fmt.Printf("Bootstrap: created admin user %q (id=%d)\n", email, adminID)
		}
	}

	if enableAPI {
		if err := storage.EnsureEnableAPI(); err != nil {
			return fmt.Errorf("bootstrap enable API: %w", err)
		}
		fmt.Println("Bootstrap: site_settings.enable_api=true")
	}

	if createKey {
		if adminID == 0 {
			if email == "" {
				return fmt.Errorf("ORDRYN_BOOTSTRAP_CREATE_API_KEY requires ORDRYN_BOOTSTRAP_ADMIN_EMAIL")
			}
			var err error
			adminID, err = storage.GetUserIDByEmail(email)
			if err != nil {
				return fmt.Errorf("bootstrap API key user: %w", err)
			}
		}
		has, err := storage.HasActiveAPIKeyNamed(adminID, bootstrapAPIKeyName)
		if err != nil {
			return fmt.Errorf("bootstrap API key lookup: %w", err)
		}
		if has {
			fmt.Printf("Bootstrap: API key %q already exists for user id=%d; not reissuing\n", bootstrapAPIKeyName, adminID)
			return nil
		}
		plaintext, record, err := storage.CreateAPIKey(adminID, bootstrapAPIKeyName)
		if err != nil {
			return fmt.Errorf("bootstrap create API key: %w", err)
		}
		fmt.Printf("Bootstrap: created API key id=%d name=%q — store this now; it will not be shown again:\n%s\n",
			record.ID, record.Name, plaintext)
	}

	return nil
}

// bootstrapSiteMode applies ORDRYN_BOOTSTRAP_SITE_MODE. Once the mode is locked a
// different value only logs a warning, so leaving the variable set never blocks startup.
func bootstrapSiteMode(raw string) error {
	mode := storage.NormalizeSiteMode(raw)
	if mode == "" {
		return fmt.Errorf("ORDRYN_BOOTSTRAP_SITE_MODE must be multi or company, got %q", raw)
	}
	current, err := storage.GetSiteMode()
	if err != nil {
		return fmt.Errorf("bootstrap site mode lookup: %w", err)
	}
	if current == mode {
		return nil
	}
	if err := storage.ChangeSiteMode(mode); err != nil {
		if errors.Is(err, storage.ErrSiteModeLocked) {
			fmt.Printf("Bootstrap: site mode is %q and locked because projects or organizations exist; ignoring ORDRYN_BOOTSTRAP_SITE_MODE=%s\n", current, mode)
			return nil
		}
		return fmt.Errorf("bootstrap site mode: %w", err)
	}
	fmt.Printf("Bootstrap: site mode set to %q\n", mode)
	return nil
}
