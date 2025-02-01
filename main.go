package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/csrf"

	"github.com/lithammer/shortuuid/v4"
	"gopkg.in/yaml.v3"
)

type Config struct {
	CSRFSecret string     `yaml:"csrf_secret"`
	Listen     string     `yaml:"listen"`
	Auth       AuthConfig `yaml:"auth"`
	Tasks      []Task     `yaml:"tasks"`
}

type TaskStats struct {
	Started  time.Time
	Ended    time.Time
	Duration time.Duration
	ExitCode int
	StdOut   string
	StdErr   string
	IsKilled bool
	Error    error
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Task struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
	Timeout int      `yaml:"timeout"` // Timeout in seconds
}

type Param struct {
	Name string `yaml:"name"`
}

var (
	config             Config
	taskListTemplate   = loadTemplates("tasklist.html")
	taskFormTemplate   = loadTemplates("taskform.html")
	taskResultTemplate = loadTemplates("result.html")
	defaultTimeout     = 15 * time.Second
)

func loadConfig() {
	data, err := os.ReadFile(getConfigFilePath())
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Failed to parse YAML: %v", err)
	}
}

func requestId(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), "request_id", uuid())
		next(w, r.WithContext(ctx))
	}
}

func middlewares(next http.HandlerFunc) http.HandlerFunc {
	return requestId(requestTimer(basicAuth(debugUser(next))))
}

func debugUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request_id := r.Context().Value("request_id").(string)
		user := r.Context().Value("user").(string)
		log.Printf("%s authenticated user is `%s`\n", request_id, user)
		next(w, r)
	}
}
func requestTimer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		request_id := r.Context().Value("request_id").(string)
		ctx := context.WithValue(r.Context(), "startTime", start)
		log.Printf("%s started\n", request_id)
		next(w, r.WithContext(ctx))
		duration := time.Since(start)
		log.Printf("%s ended. elapsed %s\n", request_id, duration)
	}
}
func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != config.Auth.Username || pass != config.Auth.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), "user", user)
		next(w, r.WithContext(ctx))
	}
}

func listTasks(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s Listing tasks. %d tasks found", r.Context().Value("request_id").(string), len(config.Tasks))
	taskListTemplate.ExecuteTemplate(w, "common.html", map[string]interface{}{
		"Title":          "Task List",
		csrf.TemplateTag: csrf.Token(r), // Embed CSRF token
		"CSRFFieldName":  "gorilla.csrf.Token",
		"Tasks":          config.Tasks,
	})
}

func executeTask(w http.ResponseWriter, r *http.Request) {
	request_id := r.Context().Value("request_id").(string)
	taskName := r.FormValue("task")
	log.Printf("%s Executing task with name `%s`", request_id, taskName)
	var selectedTask *Task

	// Find the selected task by its name
	for _, task := range config.Tasks {
		if task.Name == taskName {
			selectedTask = &task
			break
		}
	}

	// Handle task not found case
	if selectedTask == nil {
		http.Error(w, "Task not found", http.StatusBadRequest)
		return
	}

	log.Printf("%s Selected task is %s (arg count including self %d)", request_id, toJson(selectedTask.Command), len(selectedTask.Command))
	// Process the command and replace placeholders with user input
	args := make([]string, len(selectedTask.Command))
	copy(args, selectedTask.Command)

	// Loop over each argument and replace placeholders
	for i, arg := range args {
		if strings.HasPrefix(arg, "%%") {
			args[i] = arg[1:]
			continue
		}
		// Check if the argument contains placeholders like %ParamName
		for _, param := range extractParamsFromCommand(arg) {
			placeholder := "%" + param
			value := r.FormValue(param)

			log.Printf("%s   Token %s(Param %s) => Form Value `%s`\n", request_id, placeholder, param, value)
			// Check if the argument is double-escaped, and avoid replacing it
			args[i] = strings.ReplaceAll(args[i], placeholder, value)
		}
	}

	// Use task-specific timeout or default to 15 seconds
	taskTimeout := defaultTimeout
	if selectedTask.Timeout > 0 {
		taskTimeout = time.Duration(selectedTask.Timeout) * time.Second
	}

	// Run the command with the specified timeout
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	log.Printf("%s Final args: %s\n", request_id, toJson(args))
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var stats TaskStats
	stats.Started = time.Now()

	// Start the command execution
	err := cmd.Start()
	if err != nil {
		stats.IsKilled = false
		stats.Ended = time.Now()
		stats.Duration = stats.Ended.Sub(stats.Started)
		stats.ExitCode = -1
		stats.StdOut = defaultString("")
		stats.StdErr = defaultString("")
		stats.Error = errors.New("Can't start process")
		taskResultTemplate.ExecuteTemplate(w, "common.html", map[string]interface{}{
			"Title":          "Execute Task",
			csrf.TemplateTag: csrf.Token(r), // Embed CSRF token
			"CSRFFieldName":  "gorilla.csrf.Token",
			"Task":           selectedTask.Name,
			"Result":         stats,
		})
		return
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill() // Kill process if timeout reached
		stats.IsKilled = true
		stats.Ended = time.Now()
		stats.Duration = stats.Ended.Sub(stats.Started)
		stats.ExitCode = cmd.ProcessState.ExitCode()
		stats.StdOut = defaultString(stdout.String())
		stats.StdErr = defaultString(stderr.String())
		stats.Error = errors.New("Timeout killed")
		taskResultTemplate.ExecuteTemplate(w, "common.html", map[string]interface{}{
			"Title":          "Execute Task",
			csrf.TemplateTag: csrf.Token(r), // Embed CSRF token
			"CSRFFieldName":  "gorilla.csrf.Token",
			"Task":           selectedTask.Name,
			"Result":         stats,
		})
	case err := <-done:
		stats.Ended = time.Now()
		stats.ExitCode = cmd.ProcessState.ExitCode()
		stats.Duration = stats.Ended.Sub(stats.Started)
		stats.StdOut = defaultString(stdout.String())
		stats.StdErr = defaultString(stderr.String())
		stats.IsKilled = false
		if err != nil {
			stats.Error = err
		}
		taskResultTemplate.ExecuteTemplate(w, "common.html", map[string]interface{}{
			csrf.TemplateTag: csrf.Token(r), // Embed CSRF token
			"CSRFFieldName":  "gorilla.csrf.Token",
			"Title":          "Execute Task",
			"Task":           selectedTask.Name,
			"Result":         stats,
		})
	}
}

func defaultString(input string) string {
	if input == "" {
		return "N/A"
	}
	return input
}
func main() {
	loadConfig()
	CSRF := csrf.Protect(
		[]byte(config.CSRFSecret), // Load secret from environment variable
		csrf.Secure(true),         // Set to true in production (HTTPS only)
		csrf.HttpOnly(true),       // Prevent client-side JavaScript access
		csrf.Path("/"),            // Token is valid for all paths
	)
	mux := http.NewServeMux()

	mux.HandleFunc("/", middlewares(listTasks))
	mux.HandleFunc("/task", middlewares(taskForm))
	mux.HandleFunc("/execute", middlewares(executeTask))

	log.Printf("Server running on http://%s\n", config.Listen)
	err := http.ListenAndServe(config.Listen, CSRF(mux))
	log.Println(err)
}

func taskForm(w http.ResponseWriter, r *http.Request) {
	request_id := r.Context().Value("request_id").(string)
	taskName := r.FormValue("task")
	log.Printf("%s redndering task form with name `%s`\n", request_id, taskName)
	var selectedTask *Task

	// Find the selected task by its name
	for _, task := range config.Tasks {
		if task.Name == taskName {
			selectedTask = &task
			break
		}
	}

	// Handle task not found case
	if selectedTask == nil {
		http.Error(w, "Task not found", http.StatusBadRequest)
		return
	}

	// Extract parameter names from the command
	params := extractParamsFromCommandList(selectedTask.Command)

	timeout := defaultTimeout
	if selectedTask.Timeout > 0 {
		timeout = time.Duration(selectedTask.Timeout) * time.Second
	}
	// Render the form with the parameter names
	taskFormTemplate.ExecuteTemplate(w, "common.html", map[string]interface{}{
		csrf.TemplateTag: csrf.Token(r), // Embed CSRF token
		"CSRFFieldName":  "gorilla.csrf.Token",
		"Title":          "Execute Task",
		"Task":           selectedTask,
		"Timeout":        timeout,
		"Params":         params,
	})
}

// extractParamsFromCommandList scans the command and extracts parameter names (e.g., %Count, %Host)
func extractParamsFromCommandList(command []string) []string {
	var params []string
	for _, arg := range command {
		params = append(params, extractParamsFromCommand(arg)...)
	}
	return params
}

func extractParamsFromCommand(command string) []string {
	var params []string
	// Regular expression to match placeholders like %ParamName
	re := regexp.MustCompile(`^%([A-Za-z0-9_-]+)$`)
	matches := re.FindAllStringSubmatch(command, -1)

	// Extract the parameter names from the regex matches
	for _, match := range matches {
		if len(match) > 1 {
			params = append(params, match[1]) // match[1] contains the captured parameter name
		}
	}

	return params
}

func toJson(arg interface{}) string {
	jsonBytes, err := json.Marshal(arg)
	if err != nil {
		panic(err)
	}
	return string(jsonBytes)
}

func loadTemplates(name string) *template.Template {
	tmpl := template.New("")
	//, "templates/taskform.html", "templates/result.html"
	tmpl = template.Must(tmpl.ParseFiles("templates/common.html", "templates/"+name))
	return tmpl
}

func uuid() string {
	return shortuuid.New()
}

func getConfigFilePath() string {
	// Define flags for long and short versions
	configPath := flag.String("config", "", "Path to the config file (default to config.yaml/config.yml) refer to config.yaml.example for details")
	configPathShort := flag.String("c", "", "Short-hand for -config")

	// Custom help message
	flag.Usage = func() {
		fmt.Println("Usage: webaction [OPTIONS]")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}

	// Parse the command-line arguments
	flag.Parse()

	// Prioritize the short flag if both are provided
	configFile := *configPathShort
	if configFile == "" {
		configFile = *configPath
	}

	// If a config file is provided via flags, check if it exists
	if configFile != "" {
		if _, err := os.Stat(configFile); err == nil {
			return configFile
		}
		fmt.Printf("Warning: Config file '%s' not found, falling back to defaults.\n", configFile)
	}

	// Default to "./config.yaml" if it exists
	if _, err := os.Stat("./config.yaml"); err == nil {
		fmt.Printf("Default using ./config.yaml as config file\n")
		return "./config.yaml"
	}

	// Default to "./config.yml" if it exists
	if _, err := os.Stat("./config.yml"); err == nil {
		fmt.Printf("Default using ./config.yml as config file\n")
		return "./config.yml"
	}

	// If no config file is found, print an error and exit
	fmt.Println("Error: No valid config file found.")
	flag.Usage()
	os.Exit(1) // Exit the program

	return "" // This won't be reached, but ensures function signature consistency
}
