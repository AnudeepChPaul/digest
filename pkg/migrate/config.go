package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/system"

	"gopkg.in/yaml.v3"
)

var movedJanitorKeys = []struct {
	topLevel string
	option   string
}{
	{"retention_days", config.OptionGraceDays},
	{"janitor_patterns", config.OptionPatterns},
}

func mappingValue(mapping *yaml.Node, key string) (int, *yaml.Node) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return index, mapping.Content[index+1]
		}
	}
	return -1, nil
}

func setMappingValue(mapping *yaml.Node, key string, value *yaml.Node, comment string) {
	if index, _ := mappingValue(mapping, key); index >= 0 {
		mapping.Content[index+1] = value
		return
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, HeadComment: comment}, value)
}

func builtInJobNodeName(job *yaml.Node) (string, bool) {
	if job.Kind != yaml.MappingNode {
		return "", false
	}
	_, name := mappingValue(job, "name")
	if name == nil {
		return "", false
	}
	return config.BuiltInJobName(name.Value)
}

func stringNode(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if value == "~" {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

func janitorJobNode(jobs *yaml.Node) *yaml.Node {
	for _, job := range jobs.Content {
		if name, _ := builtInJobNodeName(job); name == config.JobJanitor {
			return job
		}
	}
	janitor := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{stringNode("name"), stringNode(config.JobJanitor)}}
	jobs.Content = append(jobs.Content, janitor)
	return janitor
}

func loadConfigDocument(configPath string) (*yaml.Node, *yaml.Node, error) {
	if configPath == "" {
		return nil, nil, nil
	}
	content, err := system.Read(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", configPath, err)
	}
	if len(document.Content) == 0 {
		return nil, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("parse %s: the config is not a mapping", configPath)
	}
	return &document, root, nil
}

func saveConfigDocument(configPath string, document *yaml.Node) error {
	var migrated bytes.Buffer
	encoder := yaml.NewEncoder(&migrated)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	if err := system.Write(configPath, migrated.Bytes()); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	return nil
}

func migrateJanitorOptions(configPath string, out io.Writer, dryRun bool) error {
	document, root, err := loadConfigDocument(configPath)
	if err != nil || root == nil {
		return err
	}

	var moved []string
	for _, key := range movedJanitorKeys {
		if index, _ := mappingValue(root, key.topLevel); index >= 0 {
			moved = append(moved, key.topLevel)
		}
	}
	if len(moved) == 0 {
		return nil
	}

	_, jobs := mappingValue(root, "jobs")
	if jobs == nil {
		jobs = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMappingValue(root, "jobs", jobs, "")
	}
	if jobs.Kind != yaml.SequenceNode {
		return fmt.Errorf("parse %s: jobs is not a list", configPath)
	}
	janitor := janitorJobNode(jobs)
	_, options := mappingValue(janitor, "options")
	if options == nil || options.Kind != yaml.MappingNode {
		options = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMappingValue(janitor, "options", options, "")
	}
	for _, key := range movedJanitorKeys {
		index, value := mappingValue(root, key.topLevel)
		if index < 0 {
			continue
		}
		if optionIndex, _ := mappingValue(options, key.option); optionIndex < 0 {
			setMappingValue(options, key.option, value, root.Content[index].HeadComment)
		}
		root.Content = append(root.Content[:index], root.Content[index+2:]...)
	}

	if dryRun {
		fmt.Fprintf(out, "would move %s into the janitor job's options in %s\n", strings.Join(moved, ", "), configPath)
		return nil
	}
	if err := saveConfigDocument(configPath, document); err != nil {
		return err
	}
	fmt.Fprintf(out, "moved %s into the janitor job's options in %s\n", strings.Join(moved, ", "), configPath)
	return nil
}

func templateAutomationNodes() ([]*yaml.Node, error) {
	var template yaml.Node
	if err := yaml.Unmarshal([]byte(config.DefaultConfigYAML), &template); err != nil {
		return nil, err
	}
	_, automations := mappingValue(template.Content[0], "automations")
	if automations == nil || automations.Kind != yaml.SequenceNode {
		return nil, errors.New("the config template has no automations list")
	}
	return automations.Content, nil
}

func automationNodeName(automation *yaml.Node) string {
	if automation.Kind != yaml.MappingNode {
		return ""
	}
	if _, name := mappingValue(automation, "name"); name != nil {
		return strings.ToLower(strings.TrimSpace(name.Value))
	}
	return ""
}

func migrateAutomations(configPath string, out io.Writer, dryRun bool) error {
	document, root, err := loadConfigDocument(configPath)
	if err != nil || root == nil {
		return err
	}
	_, automations := mappingValue(root, "automations")
	if automations == nil {
		return nil
	}
	if automations.Kind != yaml.SequenceNode {
		return fmt.Errorf("parse %s: automations is not a list", configPath)
	}
	templateNodes, err := templateAutomationNodes()
	if err != nil {
		return err
	}
	templateTypes := map[string]string{}
	var calendarTemplate *yaml.Node
	for _, templateNode := range templateNodes {
		name := automationNodeName(templateNode)
		if _, automationType := mappingValue(templateNode, "automation_type"); automationType != nil {
			templateTypes[name] = automationType.Value
		}
		if name == config.AutomationGoogleCalendar {
			calendarTemplate = templateNode
		}
	}

	var typed []string
	hasCalendar, prReviewIndex := false, -1
	for index, automation := range automations.Content {
		name := automationNodeName(automation)
		hasCalendar = hasCalendar || name == config.AutomationGoogleCalendar
		if name == config.AutomationPRReview && prReviewIndex < 0 {
			prReviewIndex = index
		}
		automationType, builtIn := templateTypes[name]
		if !builtIn {
			continue
		}
		if typeIndex, _ := mappingValue(automation, "automation_type"); typeIndex >= 0 {
			continue
		}
		nameIndex, _ := mappingValue(automation, "name")
		typeEntry := []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "automation_type"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: automationType},
		}
		automation.Content = append(automation.Content[:nameIndex+2], append(typeEntry, automation.Content[nameIndex+2:]...)...)
		typed = append(typed, name)
	}
	addCalendar := !hasCalendar && calendarTemplate != nil
	if addCalendar {
		if prReviewIndex < 0 {
			automations.Content = append(automations.Content, calendarTemplate)
		} else {
			automations.Content = append(automations.Content[:prReviewIndex], append([]*yaml.Node{calendarTemplate}, automations.Content[prReviewIndex:]...)...)
		}
	}
	if len(typed) == 0 && !addCalendar {
		return nil
	}

	verb := "added"
	if dryRun {
		verb = "would add"
	} else if err := saveConfigDocument(configPath, document); err != nil {
		return err
	}
	if len(typed) > 0 {
		fmt.Fprintf(out, "%s automation_type to %s in %s\n", verb, strings.Join(typed, ", "), configPath)
	}
	if addCalendar {
		fmt.Fprintf(out, "%s the google calendar automation to %s\n", verb, configPath)
	}
	return nil
}

var jobCommandKeys = []string{"command", "dry-run-command", "cmd", "dry_run_command"}

func rootsInCommand(command string) []string {
	var roots []string
	words := strings.Fields(command)
	for index, word := range words {
		switch {
		case (word == "--root" || word == "-root") && index+1 < len(words):
			roots = append(roots, words[index+1])
		case strings.HasPrefix(word, "--root="), strings.HasPrefix(word, "-root="):
			roots = append(roots, word[strings.Index(word, "=")+1:])
		}
	}
	return roots
}

func migrateBuiltInJob(job *yaml.Node, name string) bool {
	changed := false
	if _, nameNode := mappingValue(job, "name"); nameNode.Value != name {
		nameNode.Value = name
		changed = true
	}
	var roots []string
	for _, key := range []string{"command", "cmd", "dry-run-command", "dry_run_command"} {
		if _, value := mappingValue(job, key); value != nil && len(roots) == 0 {
			roots = rootsInCommand(value.Value)
		}
	}
	for _, key := range jobCommandKeys {
		if index, _ := mappingValue(job, key); index >= 0 {
			job.Content = append(job.Content[:index], job.Content[index+2:]...)
			changed = true
		}
	}
	if len(roots) == 0 {
		return changed
	}
	_, options := mappingValue(job, "options")
	if options == nil || options.Kind != yaml.MappingNode {
		options = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMappingValue(job, "options", options, "")
	}
	if index, _ := mappingValue(options, config.OptionRoots); index < 0 {
		rootsNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, root := range roots {
			rootsNode.Content = append(rootsNode.Content, stringNode(root))
		}
		setMappingValue(options, config.OptionRoots, rootsNode, "")
	}
	return true
}

func migrateBuiltInJobs(configPath string, out io.Writer, dryRun bool) error {
	document, root, err := loadConfigDocument(configPath)
	if err != nil || root == nil {
		return err
	}
	_, jobs := mappingValue(root, "jobs")
	if jobs == nil {
		return nil
	}
	if jobs.Kind != yaml.SequenceNode {
		return fmt.Errorf("parse %s: jobs is not a list", configPath)
	}
	var migrated []string
	for _, job := range jobs.Content {
		if name, builtIn := builtInJobNodeName(job); builtIn && migrateBuiltInJob(job, name) && !slices.Contains(migrated, name) {
			migrated = append(migrated, name)
		}
	}
	if len(migrated) == 0 {
		return nil
	}
	verb := "removed"
	if dryRun {
		verb = "would remove"
	} else if err := saveConfigDocument(configPath, document); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s the commands of %s in %s; their --root values are now the roots option\n", verb, strings.Join(migrated, ", "), configPath)
	return nil
}
