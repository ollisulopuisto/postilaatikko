package settings

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// KeyEmailTemplates stores reusable full-message templates locally.
const KeyEmailTemplates = "email_templates"

// EmailTemplate is a reusable subject/body pair for composing messages.
type EmailTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// ListEmailTemplates returns saved templates in creation order.
func (s *Store) ListEmailTemplates() ([]EmailTemplate, error) {
	value, err := s.Get(KeyEmailTemplates)
	if err != nil {
		return nil, err
	}
	templates := []EmailTemplate{}
	if value == "" {
		return templates, nil
	}
	if err := json.Unmarshal([]byte(value), &templates); err != nil {
		s.log.Warn().Err(err).Msg("Failed to parse email templates, using empty list")
		return []EmailTemplate{}, nil
	}
	return templates, nil
}

// SaveEmailTemplate creates or updates a template. It must contain a subject or body.
func (s *Store) SaveEmailTemplate(template EmailTemplate) (EmailTemplate, error) {
	template.Name = strings.TrimSpace(template.Name)
	template.Subject = strings.TrimSpace(template.Subject)
	if template.Name == "" {
		return EmailTemplate{}, fmt.Errorf("email template name is required")
	}
	if template.Subject == "" && strings.TrimSpace(template.Body) == "" {
		return EmailTemplate{}, fmt.Errorf("email template subject or body is required")
	}

	templates, err := s.ListEmailTemplates()
	if err != nil {
		return EmailTemplate{}, err
	}
	if template.ID == "" {
		template.ID = uuid.NewString()
		templates = append(templates, template)
		return template, s.writeEmailTemplates(templates)
	}
	for i := range templates {
		if templates[i].ID != template.ID {
			continue
		}
		templates[i] = template
		return template, s.writeEmailTemplates(templates)
	}
	return EmailTemplate{}, fmt.Errorf("email template not found: %s", template.ID)
}

// DeleteEmailTemplate removes a template by ID; deleting a missing ID is harmless.
func (s *Store) DeleteEmailTemplate(id string) error {
	templates, err := s.ListEmailTemplates()
	if err != nil {
		return err
	}
	kept := make([]EmailTemplate, 0, len(templates))
	for _, template := range templates {
		if template.ID != id {
			kept = append(kept, template)
		}
	}
	return s.writeEmailTemplates(kept)
}

func (s *Store) writeEmailTemplates(templates []EmailTemplate) error {
	data, err := json.Marshal(templates)
	if err != nil {
		return fmt.Errorf("failed to marshal email templates: %w", err)
	}
	return s.Set(KeyEmailTemplates, string(data))
}
