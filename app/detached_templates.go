package app

import "github.com/hkdb/aerion/internal/settings"

// ListEmailTemplates reads templates from the parent desktop's local settings.
func (c *ComposerApp) ListEmailTemplates() ([]settings.EmailTemplate, error) {
	return callParentValue[[]settings.EmailTemplate](c, "ListEmailTemplates")
}

// SaveEmailTemplate validates and saves a template through the parent desktop.
func (c *ComposerApp) SaveEmailTemplate(template settings.EmailTemplate) (settings.EmailTemplate, error) {
	return callParentValue[settings.EmailTemplate](c, "SaveEmailTemplate", template)
}

// DeleteEmailTemplate removes a template through the parent desktop.
func (c *ComposerApp) DeleteEmailTemplate(id string) error {
	_, err := c.callParent("DeleteEmailTemplate", id)
	return err
}
