package services

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// The Set*Active methods flip an object's live switch and nothing else. They
// back the dedicated activate/deactivate routes (annotated with the publish
// permission) so a reviewer role can release an object without holding write,
// and they are the one place the switch is written outside a full update, so
// object hooks and system events fire exactly as they do for an update.

// SetLLMActive sets the active flag of an LLM.
func (s *Service) SetLLMActive(id uint, active bool, userID uint) (*models.LLM, error) {
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return nil, err
	}
	if llm.Active == active {
		return llm, nil
	}
	llm.Active = active
	if err := s.runBeforeUpdateHook(ObjectTypeLLM, llm, userID); err != nil {
		return nil, err
	}
	if err := s.DB.Model(&models.LLM{}).Where("id = ?", id).Update("active", active).Error; err != nil {
		return nil, err
	}
	s.runAfterUpdateHook(ObjectTypeLLM, llm, userID)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitLLMUpdated(llm, llm.ID, userID)
	}
	return llm, nil
}

// SetToolActive sets the active flag of a tool.
func (s *Service) SetToolActive(id uint, active bool, userID uint) (*models.Tool, error) {
	tool, err := s.GetToolByID(id)
	if err != nil {
		return nil, err
	}
	if tool.Active == active {
		return tool, nil
	}
	tool.Active = active
	if err := s.runBeforeUpdateHook(ObjectTypeTool, tool, userID); err != nil {
		return nil, err
	}
	if err := s.DB.Model(&models.Tool{}).Where("id = ?", id).Update("active", active).Error; err != nil {
		return nil, err
	}
	s.runAfterUpdateHook(ObjectTypeTool, tool, userID)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitToolUpdated(tool, tool.ID, userID)
	}
	return tool, nil
}

// SetDatasourceActive sets the active flag of a datasource.
func (s *Service) SetDatasourceActive(id uint, active bool, userID uint) (*models.Datasource, error) {
	ds, err := s.GetDatasourceByID(id)
	if err != nil {
		return nil, err
	}
	if ds.Active == active {
		return ds, nil
	}
	ds.Active = active
	if err := s.runBeforeUpdateHook(ObjectTypeDatasource, ds, userID); err != nil {
		return nil, err
	}
	if err := s.DB.Model(&models.Datasource{}).Where("id = ?", id).Update("active", active).Error; err != nil {
		return nil, err
	}
	s.runAfterUpdateHook(ObjectTypeDatasource, ds, userID)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitDatasourceUpdated(ds, ds.ID, userID)
	}
	return ds, nil
}

// SetAppActive sets the is_active flag of an app. Apps have no object hooks.
func (s *Service) SetAppActive(id uint, active bool, userID uint) (*models.App, error) {
	app, err := s.GetAppByID(id)
	if err != nil {
		return nil, err
	}
	if app.IsActive == active {
		return app, nil
	}
	if err := s.DB.Model(&models.App{}).Where("id = ?", id).Update("is_active", active).Error; err != nil {
		return nil, err
	}
	app.IsActive = active
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(app, app.ID, userID)
	}
	return app, nil
}

func (s *Service) runBeforeUpdateHook(objectType ObjectType, object interface{}, userID uint) error {
	if s.HookManager == nil {
		return nil
	}
	result, err := s.HookManager.ExecuteHooks(context.Background(), objectType, HookBeforeUpdate, object, uint32(userID))
	if err != nil {
		return fmt.Errorf("hook execution failed: %w", err)
	}
	if !result.Allowed {
		return fmt.Errorf("operation rejected by plugin: %s", result.RejectionReason)
	}
	return nil
}

func (s *Service) runAfterUpdateHook(objectType ObjectType, object interface{}, userID uint) {
	if s.HookManager == nil {
		return
	}
	if _, err := s.HookManager.ExecuteHooks(context.Background(), objectType, HookAfterUpdate, object, uint32(userID)); err != nil {
		logger.Warn(fmt.Sprintf("After-update hooks failed: %v", err))
	}
}
