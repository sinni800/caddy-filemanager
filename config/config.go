package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/webdav"

	"github.com/mholt/caddy"
	"github.com/mholt/caddy/caddyhttp/httpserver"
)

// Config is a configuration for browsing in a particular path.
type Config struct {
	*User
	BaseURL     string
	AbsoluteURL string
	AddrPath    string
	Token       string // Anti CSRF token
	HugoEnabled bool   // Enables the Hugo plugin for File Manager
	Users       map[string]*User
	WebDavURL   string
	CurrentUser *User
}

// Rule is a dissalow/allow rule
type Rule struct {
	Regex  bool
	Allow  bool
	Path   string
	Regexp *regexp.Regexp
}

// Parse parses the configuration set by the user so it can
// be used by the middleware
func Parse(c *caddy.Controller) ([]Config, error) {
	var (
		configs []Config
		err     error
		user    *User
	)

	appendConfig := func(cfg Config) error {
		for _, c := range configs {
			if c.Scope == cfg.Scope {
				return fmt.Errorf("duplicate file managing config for %s", c.Scope)
			}
		}
		configs = append(configs, cfg)
		return nil
	}

	for c.Next() {
		// Initialize the configuration with the default settings
		cfg := Config{User: &User{}}
		cfg.Scope = "."
		cfg.FileSystem = webdav.Dir(cfg.Scope)
		cfg.BaseURL = ""
		cfg.FrontMatter = "yaml"
		cfg.HugoEnabled = false
		cfg.Users = map[string]*User{}
		cfg.AllowCommands = true
		cfg.AllowEdit = true
		cfg.AllowNew = true
		cfg.Commands = []string{"git", "svn", "hg"}
		cfg.Rules = []*Rule{{
			Regex:  true,
			Allow:  false,
			Regexp: regexp.MustCompile("\\/\\..+"),
		}}

		// Get the baseURL
		args := c.RemainingArgs()

		if len(args) > 0 {
			cfg.BaseURL = args[0]
		}

		cfg.BaseURL = strings.TrimPrefix(cfg.BaseURL, "/")
		cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
		cfg.BaseURL = "/" + cfg.BaseURL
		cfg.WebDavURL = cfg.BaseURL + "webdav"

		if cfg.BaseURL == "/" {
			cfg.BaseURL = ""
		}

		// Set the first user, the global user
		user = cfg.User

		for c.NextBlock() {
			switch c.Val() {
			case "frontmatter":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.FrontMatter = c.Val()
				if user.FrontMatter != "yaml" && user.FrontMatter != "json" && user.FrontMatter != "toml" {
					return configs, c.Err("frontmatter type not supported")
				}
			case "webdav":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				prefix := c.Val()
				prefix = strings.TrimPrefix(prefix, "/")
				prefix = strings.TrimSuffix(prefix, "/")
				prefix = cfg.BaseURL + "/" + prefix
				cfg.WebDavURL = prefix
			case "show":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.Scope = c.Val()
				user.Scope = strings.TrimSuffix(user.Scope, "/")
				user.FileSystem = webdav.Dir(user.Scope)
			case "styles":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				var tplBytes []byte
				tplBytes, err = ioutil.ReadFile(c.Val())
				if err != nil {
					return configs, err
				}
				user.StyleSheet = string(tplBytes)
			case "allow_new":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.AllowNew, err = strconv.ParseBool(c.Val())
				if err != nil {
					return configs, err
				}
			case "allow_edit":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.AllowEdit, err = strconv.ParseBool(c.Val())
				if err != nil {
					return configs, err
				}
			case "allow_commands":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.AllowCommands, err = strconv.ParseBool(c.Val())
				if err != nil {
					return configs, err
				}
			case "allow_command":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				user.Commands = append(user.Commands, c.Val())
			case "block_command":
				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				index := 0

				for i, val := range user.Commands {
					if val == c.Val() {
						index = i
					}
				}

				user.Commands = append(user.Commands[:index], user.Commands[index+1:]...)
			case "allow", "allow_r", "block", "block_r":
				ruleType := c.Val()

				if !c.NextArg() {
					return configs, c.ArgErr()
				}

				if c.Val() == "dotfiles" && !strings.HasSuffix(ruleType, "_r") {
					ruleType += "_r"
				}

				rule := &Rule{
					Allow: ruleType == "allow" || ruleType == "allow_r",
					Regex: ruleType == "allow_r" || ruleType == "block_r",
				}

				if rule.Regex && c.Val() == "dotfiles" {
					rule.Regexp = regexp.MustCompile("\\/\\..+")
				} else if rule.Regex {
					rule.Regexp = regexp.MustCompile(c.Val())
				} else {
					rule.Path = c.Val()
				}

				user.Rules = append(user.Rules, rule)
			// NEW USER BLOCK?
			default:
				val := c.Val()

				// Checks if it's a new user
				if !strings.HasSuffix(val, ":") {
					fmt.Println("Unknown option " + val)
				}

				// Get the username, sets the current user, and initializes it
				val = strings.TrimSuffix(val, ":")
				cfg.Users[val] = &User{}

				// Initialize the new user
				user = cfg.Users[val]
				user.AllowCommands = cfg.AllowCommands
				user.AllowEdit = cfg.AllowEdit
				user.AllowNew = cfg.AllowEdit
				user.Commands = cfg.Commands
				user.FrontMatter = cfg.FrontMatter
				user.Scope = cfg.Scope
				user.FileSystem = cfg.FileSystem
				user.Rules = cfg.Rules
				user.StyleSheet = cfg.StyleSheet
			}
		}

		cfg.Handler = &webdav.Handler{
			Prefix:     cfg.WebDavURL,
			FileSystem: cfg.FileSystem,
			LockSystem: webdav.NewMemLS(),
		}

		caddyConf := httpserver.GetConfig(c)
		cfg.AbsoluteURL = strings.TrimSuffix(caddyConf.Addr.Path, "/") + "/" + cfg.BaseURL
		cfg.AbsoluteURL = strings.Replace(cfg.AbsoluteURL, "//", "/", -1)
		cfg.AbsoluteURL = strings.TrimSuffix(cfg.AbsoluteURL, "/")
		cfg.AddrPath = strings.TrimSuffix(caddyConf.Addr.Path, "/")
		if err := appendConfig(cfg); err != nil {
			return configs, err
		}
	}

	return configs, nil
}
