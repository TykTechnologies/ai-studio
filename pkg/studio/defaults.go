package studio

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
)

// ensureDefaults ensures default group and catalogues exist and are linked
func ensureDefaults(db *gorm.DB, skipLLMDefaults, skipFilterDefaults bool) error {
	logger.Info("Ensuring default group and catalogues exist...")

	// Get or create Default group
	defaultGroup, err := models.GetOrCreateDefaultGroup(db)
	if err != nil {
		return fmt.Errorf("failed to ensure default group: %w", err)
	}
	logger.Infof("Default group ensured (ID: %d, Name: %s)", defaultGroup.ID, defaultGroup.Name)

	// Get or create Default LLM catalogue
	defaultCatalogue, err := models.GetOrCreateDefaultCatalogue(db)
	if err != nil {
		return fmt.Errorf("failed to ensure default catalogue: %w", err)
	}
	logger.Infof("Default LLM catalogue ensured (ID: %d, Name: %s)", defaultCatalogue.ID, defaultCatalogue.Name)

	// Get or create Default data catalogue
	defaultDataCatalogue, err := models.GetOrCreateDefaultDataCatalogue(db)
	if err != nil {
		return fmt.Errorf("failed to ensure default data catalogue: %w", err)
	}
	logger.Infof("Default data catalogue ensured (ID: %d, Name: %s)", defaultDataCatalogue.ID, defaultDataCatalogue.Name)

	// Get or create Default tool catalogue
	defaultToolCatalogue, err := models.GetOrCreateDefaultToolCatalogue(db)
	if err != nil {
		return fmt.Errorf("failed to ensure default tool catalogue: %w", err)
	}
	logger.Infof("Default tool catalogue ensured (ID: %d, Name: %s)", defaultToolCatalogue.ID, defaultToolCatalogue.Name)

	// Link catalogues to default group if not already linked
	if err := linkCatalogueToGroup(db, defaultGroup, defaultCatalogue); err != nil {
		return fmt.Errorf("failed to link LLM catalogue to default group: %w", err)
	}

	if err := linkDataCatalogueToGroup(db, defaultGroup, defaultDataCatalogue); err != nil {
		return fmt.Errorf("failed to link data catalogue to default group: %w", err)
	}

	if err := linkToolCatalogueToGroup(db, defaultGroup, defaultToolCatalogue); err != nil {
		return fmt.Errorf("failed to link tool catalogue to default group: %w", err)
	}

	// Seed default LLM settings if table is empty (for quick start UX)
	if err := models.GetOrCreateDefaultLLMSettings(db); err != nil {
		return fmt.Errorf("failed to create default LLM settings: %w", err)
	}
	logger.Info("Default LLM settings checked/initialized")

	// Seed the built-in client tools (generative UI "present") so chat rooms
	// can pick them as defaults without an administrator authoring them.
	if err := models.GetOrCreateDefaultClientTools(db); err != nil {
		return fmt.Errorf("failed to create default client tools: %w", err)
	}
	logger.Info("Default client tools checked/initialized")

	// Seed the default governed metadata vocabularies and "Governance Core" schema
	// (Enterprise only; advisory mode so existing objects are never blocked).
	if governed_metadata.IsEnterpriseAvailable() {
		if err := governed_metadata.NewService(db, governed_metadata.Deps{}).EnsureDefaults(); err != nil {
			logger.Warnf("Failed to seed default governed metadata schema: %v", err)
		} else {
			logger.Info("Governed metadata defaults checked/initialized")
		}
	}

	// Upgrade any legacy-format encrypted secrets to authenticated encryption.
	// Runs in the background so scrypt's deliberate cost never delays startup;
	// decrypt handles both formats, so reads are correct while it runs.
	safe.Go("legacy secret re-encryption", func() {
		logger.Info("Starting background re-encryption of legacy secrets (if any)")
		if migrated, err := secrets.ReencryptLegacySecrets(db); err != nil {
			logger.Errorf("Failed to re-encrypt legacy secrets: %v", err)
		} else if migrated > 0 {
			logger.Infof("Background migration complete: re-encrypted %d legacy secret(s) to AES-GCM format", migrated)
		} else {
			logger.Info("No legacy-format secrets to re-encrypt")
		}
	})

	// Seed default secrets and LLM configurations if not disabled
	if !skipLLMDefaults {
		if err := secrets.GetOrCreateDefaultSecrets(db); err != nil {
			return fmt.Errorf("failed to create default secrets: %w", err)
		}
		logger.Info("Default secrets checked/initialized")

		if err := models.GetOrCreateDefaultLLMs(db); err != nil {
			return fmt.Errorf("failed to create default LLM configurations: %w", err)
		}
		logger.Info("Default LLM configurations checked/initialized")
	}

	// Seed the default guardrail filters (Enterprise, where filters execute).
	// They are created unattached, so nothing is enforced until an
	// administrator attaches one. AppConf.SkipFilterDefaults
	// (SKIP_FILTER_DEFAULTS=true) skips this.
	if config.IsEnterprise() && !skipFilterDefaults {
		if err := models.GetOrCreateDefaultFilters(db); err != nil {
			return fmt.Errorf("failed to create default guardrail filters: %w", err)
		}
		logger.Info("Default guardrail filters checked/initialized")
	}

	logger.Info("Default group and catalogues successfully initialized and linked")
	return nil
}

// linkCatalogueToGroup links an LLM catalogue to a group if not already linked
func linkCatalogueToGroup(db *gorm.DB, group *models.Group, catalogue *models.Catalogue) error {
	count := db.Model(group).Where("catalogue_id = ?", catalogue.ID).Association("Catalogues").Count()
	if count == 0 {
		return db.Model(group).Association("Catalogues").Append(catalogue)
	}
	return nil
}

// linkDataCatalogueToGroup links a data catalogue to a group if not already linked
func linkDataCatalogueToGroup(db *gorm.DB, group *models.Group, catalogue *models.DataCatalogue) error {
	count := db.Model(group).Where("data_catalogue_id = ?", catalogue.ID).Association("DataCatalogues").Count()
	if count == 0 {
		return db.Model(group).Association("DataCatalogues").Append(catalogue)
	}
	return nil
}

// linkToolCatalogueToGroup links a tool catalogue to a group if not already linked
func linkToolCatalogueToGroup(db *gorm.DB, group *models.Group, catalogue *models.ToolCatalogue) error {
	count := db.Model(group).Where("tool_catalogue_id = ?", catalogue.ID).Association("ToolCatalogues").Count()
	if count == 0 {
		return db.Model(group).Association("ToolCatalogues").Append(catalogue)
	}
	return nil
}
