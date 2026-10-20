//go:build windows || linux

package desktop

import (
	"opsbuddy/internal/app"
	"opsbuddy/internal/pet"
)

// Bind user operations only, never lifecycle callbacks or the internal handler.
type NativeAPI struct{ controller *Controller }

func Bindings(c *Controller) *NativeAPI { return &NativeAPI{controller: c} }
func (n *NativeAPI) PickFiles(projectID string) ([]app.Asset, error) {
	return n.controller.PickFiles(projectID)
}
func (n *NativeAPI) PickFolder(projectID string) ([]app.Asset, error) {
	return n.controller.PickFolder(projectID)
}
func (n *NativeAPI) PickScheduleDirectory(projectID string) (app.DirectoryGrant, error) {
	return n.controller.PickScheduleDirectory(projectID)
}
func (n *NativeAPI) SaveAsset(assetID string) (string, error) { return n.controller.SaveAsset(assetID) }
func (n *NativeAPI) OpenAsset(assetID string) error           { return n.controller.OpenAsset(assetID) }
func (n *NativeAPI) RevealAsset(assetID string) error         { return n.controller.RevealAsset(assetID) }
func (n *NativeAPI) OpenDataFolder() error                    { return n.controller.OpenDataFolder() }
func (n *NativeAPI) Info() map[string]string                  { return n.controller.Info() }
func (n *NativeAPI) ImportProject() (app.ProjectImportResult, error) {
	return n.controller.ImportProject()
}
func (n *NativeAPI) Ready() error                        { return n.controller.Ready() }
func (n *NativeAPI) ReportSmoke(jsonReport string) error { return n.controller.ReportSmoke(jsonReport) }
func (n *NativeAPI) PetSettings() (pet.Settings, error)  { return n.controller.PetSettings() }
func (n *NativeAPI) ConfigurePet(settings pet.Settings) (pet.Settings, error) {
	return n.controller.ConfigurePet(settings)
}
func (n *NativeAPI) PetDiagnostics() pet.Diagnostics { return n.controller.PetDiagnostics() }
