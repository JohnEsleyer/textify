package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goTestSource = `package sample

type Config struct {
	Name   string
	Client *HTTPClient
	Peers  []*Peer
}

type HTTPClient struct{}

type Peer struct{}

func (c *Config) Load() {
	c.fetch()
	helper(c.Name)
}

func (c *Config) fetch() {
	_ = c.Client
}

func helper(name string) string {
	return name
}

func A() {
	B()
}

func B() {
	C()
}

func C() {
	helper("x")
}
`

func TestParseGoFile(t *testing.T) {
	symbols, err := ParseGoFile("sample.go", []byte(goTestSource))
	if err != nil {
		t.Fatalf("ParseGoFile failed: %v", err)
	}

	config := findSymbol(t, symbols, "Config", "")
	if config.Kind != KindStruct {
		t.Errorf("Config kind = %s, want struct", config.Kind)
	}
	if config.StartLine != 3 || config.EndLine != 7 {
		t.Errorf("Config lines = %d-%d, want 3-7", config.StartLine, config.EndLine)
	}
	if !contains(config.References, "HTTPClient") || !contains(config.References, "Peer") {
		t.Errorf("Config references = %v, want HTTPClient and Peer", config.References)
	}

	load := findSymbol(t, symbols, "Load", "*Config")
	if load.Kind != KindMethod {
		t.Errorf("Load kind = %s, want method", load.Kind)
	}
	if load.FullName() != "(*Config).Load" {
		t.Errorf("Load full name = %s", load.FullName())
	}
	if !contains(load.Calls, "c.fetch") || !contains(load.Calls, "helper") {
		t.Errorf("Load calls = %v", load.Calls)
	}
	if load.StartLine != 13 {
		t.Errorf("Load start line = %d, want 13", load.StartLine)
	}
}

func TestGoIndexDefinitionsAndFlow(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "sample.go", goTestSource)

	idx, err := BuildIndex(dir, ParseLanguages("go"))
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	matches := idx.FindDefinition("Config.Load", "")
	if len(matches) != 1 || matches[0].Name != "Load" {
		t.Fatalf("Config.Load matches = %+v", matches)
	}

	configs := idx.FindDefinition("Config", "")
	if len(configs) == 0 || configs[0].Kind != KindStruct {
		t.Fatalf("Config matches = %+v", configs)
	}

	flow := idx.TraceFlow("A", 5)
	for _, want := range []string{"A()", "B()", "C()", "helper()"} {
		if !strings.Contains(flow, want) {
			t.Errorf("TraceFlow(A) missing %q:\n%s", want, flow)
		}
	}
	if !strings.Contains(flow, " -> ") {
		t.Errorf("TraceFlow(A) should contain an arrow chain:\n%s", flow)
	}

	typeFlow := idx.TraceFlow("Config", 5)
	for _, want := range []string{"Attached Methods", "(*Config).Load()", "Type Dependency Flow", "Config -> HTTPClient"} {
		if !strings.Contains(typeFlow, want) {
			t.Errorf("TraceFlow(Config) missing %q:\n%s", want, typeFlow)
		}
	}
}

const tsTestSource = `export interface User {
  id: string;
}

export type UserId = string;

export class UserService {
  constructor(private repo: Repo) {}

  async loadUser(id: string): Promise<User> {
    const u = await this.repo.find(id);
    return this.decorate(u);
  }

  private decorate(u: User): User {
    return validate(u);
  }
}

export class AdminService extends UserService implements Logger {
  run(): void {
    this.loadUser("x");
  }
}

export function bootstrap() {
  const service = new AdminService();
  service.run();
}

export const helper = (x: number) => x + 1;
`

func TestParseTypeScriptFile(t *testing.T) {
	symbols, err := ParseTypeScriptFile("service.ts", []byte(tsTestSource))
	if err != nil {
		t.Fatalf("ParseTypeScriptFile failed: %v", err)
	}

	user := findSymbol(t, symbols, "User", "")
	if user.Kind != KindInterface {
		t.Errorf("User kind = %s, want interface", user.Kind)
	}

	userID := findSymbol(t, symbols, "UserId", "")
	if userID.Kind != KindType {
		t.Errorf("UserId kind = %s, want type", userID.Kind)
	}

	service := findSymbol(t, symbols, "UserService", "")
	if service.Kind != KindClass {
		t.Errorf("UserService kind = %s, want class", service.Kind)
	}
	if service.StartLine != 7 || service.EndLine != 18 {
		t.Errorf("UserService lines = %d-%d, want 7-18", service.StartLine, service.EndLine)
	}

	constructor := findSymbol(t, symbols, "constructor", "UserService")
	if constructor.Kind != KindMethod {
		t.Errorf("constructor kind = %s, want method", constructor.Kind)
	}

	load := findSymbol(t, symbols, "loadUser", "UserService")
	if load.Kind != KindMethod {
		t.Errorf("loadUser kind = %s, want method", load.Kind)
	}
	if !contains(load.Calls, "repo.find") || !contains(load.Calls, "this.decorate") {
		t.Errorf("loadUser calls = %v", load.Calls)
	}
	if load.StartLine != 10 || load.EndLine != 13 {
		t.Errorf("loadUser lines = %d-%d, want 10-13", load.StartLine, load.EndLine)
	}

	admin := findSymbol(t, symbols, "AdminService", "")
	if !contains(admin.References, "UserService") || !contains(admin.References, "Logger") {
		t.Errorf("AdminService references = %v", admin.References)
	}

	bootstrap := findSymbol(t, symbols, "bootstrap", "")
	if !contains(bootstrap.Calls, "AdminService") || !contains(bootstrap.Calls, "service.run") {
		t.Errorf("bootstrap calls = %v", bootstrap.Calls)
	}

	helper := findSymbol(t, symbols, "helper", "")
	if helper.Kind != KindFunction {
		t.Errorf("helper kind = %s, want function", helper.Kind)
	}
}

func TestTypeScriptIndexFlow(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "service.ts", tsTestSource)

	idx, err := BuildIndex(dir, ParseLanguages("ts"))
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	matches := idx.FindDefinition("UserService.loadUser", "")
	if len(matches) != 1 || matches[0].Name != "loadUser" {
		t.Fatalf("UserService.loadUser matches = %+v", matches)
	}

	flow := idx.TraceFlow("loadUser", 5)
	for _, want := range []string{"(UserService).loadUser()", "(UserService).decorate()", "validate()"} {
		if !strings.Contains(flow, want) {
			t.Errorf("TraceFlow(loadUser) missing %q:\n%s", want, flow)
		}
	}

	typeFlow := idx.TraceFlow("AdminService", 5)
	for _, want := range []string{"Attached Methods", "(AdminService).run()", "AdminService -> UserService", "AdminService -> Logger"} {
		if !strings.Contains(typeFlow, want) {
			t.Errorf("TraceFlow(AdminService) missing %q:\n%s", want, typeFlow)
		}
	}
}

func TestParseLanguages(t *testing.T) {
	langs := ParseLanguages("go,ts")
	if !langs[LangGo] || !langs[LangTypeScript] {
		t.Errorf("ParseLanguages(go,ts) = %v", langs)
	}
	if len(ParseLanguages("javascript")) != 1 || !ParseLanguages("javascript")[LangTypeScript] {
		t.Errorf("ParseLanguages(javascript) should select TypeScript")
	}
}

func findSymbol(t *testing.T, symbols []Symbol, name, receiver string) Symbol {
	t.Helper()
	for _, sym := range symbols {
		if sym.Name != name {
			continue
		}
		if receiver == "" || strings.Contains(sym.Receiver, receiver) {
			return sym
		}
	}
	t.Fatalf("symbol %q (receiver %q) not found in %+v", name, receiver, symbols)
	return Symbol{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
