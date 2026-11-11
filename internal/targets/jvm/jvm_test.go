package jvm

import (
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/targets"
)

const javaSample = `package com.example.demo;

import java.util.List;
import java.util.Optional;

@Service
public class UserService extends BaseService implements IUserService {
    private final UserRepository userRepository;

    public UserService(UserRepository userRepository) {
        this.userRepository = userRepository;
    }

    @Override
    public Optional<User> findById(Long id) {
        validate(id);
        return userRepository.findById(id);
    }

    private void validate(Long id) {
        if (id == null) throw new IllegalArgumentException();
    }
}

public interface UserRepository {
    Optional<User> findById(Long id);
}

public record UserDto(String id, String name) implements java.io.Serializable {}

public enum Status {
    ACTIVE, INACTIVE;
}
`

func TestJavaTarget(t *testing.T) {
	target := &JavaTarget{}
	if !target.Match("src/main/java/UserService.java") {
		t.Errorf("JavaTarget should match .java file")
	}
	if target.Match("src/main/kotlin/UserService.kt") {
		t.Errorf("JavaTarget should not match .kt file")
	}

	syms, err := target.Parse("UserService.java", []byte(javaSample))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	var foundClass, foundIface, foundRecord, foundEnum, foundMethod bool
	for _, s := range syms {
		switch s.Name {
		case "UserService":
			if s.Kind == targets.KindClass {
				foundClass = true
			}
		case "UserRepository":
			if s.Kind == targets.KindInterface {
				foundIface = true
			}
		case "UserDto":
			if s.Kind == targets.KindClass {
				foundRecord = true
			}
		case "Status":
			if s.Kind == targets.KindClass {
				foundEnum = true
			}
		case "findById":
			if s.Kind == targets.KindMethod && s.Receiver == "UserService" {
				foundMethod = true
				if len(s.Calls) == 0 || s.Calls[0] != "validate" {
					t.Errorf("expected findById to call validate, got calls: %v", s.Calls)
				}
			}
		}
	}

	if !foundClass {
		t.Errorf("UserService class symbol not found")
	}
	if !foundIface {
		t.Errorf("UserRepository interface symbol not found")
	}
	if !foundRecord {
		t.Errorf("UserDto record symbol not found")
	}
	if !foundEnum {
		t.Errorf("Status enum symbol not found")
	}
	if !foundMethod {
		t.Errorf("UserService.findById method symbol not found")
	}

	sigs := target.ExtractSignatures("UserService.java", javaSample)
	if !strings.Contains(sigs, "public class UserService") {
		t.Errorf("signatures missing UserService: %s", sigs)
	}
	if !strings.Contains(sigs, "public Optional<User> findById(Long id) { /* ... */ }") {
		t.Errorf("signatures should collapse findById body: %s", sigs)
	}
	if strings.Contains(sigs, "throw new IllegalArgumentException") {
		t.Errorf("signatures should not contain method body implementation: %s", sigs)
	}
	if !strings.Contains(sigs, "public interface UserRepository") {
		t.Errorf("signatures missing UserRepository: %s", sigs)
	}
	if !strings.Contains(sigs, "ACTIVE, INACTIVE;") {
		t.Errorf("signatures should preserve enum constants: %s", sigs)
	}
}

const kotlinSample = `package com.example.demo

import java.io.Serializable

data class User(
    val id: String,
    val name: String
) : Serializable

interface UserRepository {
    fun findById(id: String): User?
}

class UserService(
    private val repo: UserRepository
) {
    fun getUser(id: String): User {
        validate(id)
        return repo.findById(id) ?: throw Exception("Not found")
    }

    private fun validate(id: String): Boolean = id.isNotEmpty()
}

fun String.toSlug(): String {
    return lowercase().replace(" ", "-")
}
`

func TestKotlinTarget(t *testing.T) {
	target := &KotlinTarget{}
	if !target.Match("src/main/kotlin/App.kt") {
		t.Errorf("KotlinTarget should match .kt")
	}
	if !target.Match("build.gradle.kts") {
		t.Errorf("KotlinTarget should match .kts")
	}

	syms, err := target.Parse("App.kt", []byte(kotlinSample))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	var foundDataClass, foundService, foundMethod, foundExtFunc bool
	for _, s := range syms {
		switch s.Name {
		case "User":
			if s.Kind == targets.KindClass {
				foundDataClass = true
			}
		case "UserService":
			if s.Kind == targets.KindClass {
				foundService = true
			}
		case "getUser":
			if s.Kind == targets.KindMethod && s.Receiver == "UserService" {
				foundMethod = true
				if len(s.Calls) == 0 || s.Calls[0] != "validate" {
					t.Errorf("expected getUser to call validate, got calls: %v", s.Calls)
				}
			}
		case "toSlug":
			if s.Kind == targets.KindFunction && s.Receiver == "String" {
				foundExtFunc = true
			}
		}
	}

	if !foundDataClass {
		t.Errorf("User data class symbol not found")
	}
	if !foundService {
		t.Errorf("UserService class symbol not found")
	}
	if !foundMethod {
		t.Errorf("UserService.getUser method symbol not found")
	}
	if !foundExtFunc {
		t.Errorf("String.toSlug extension function symbol not found")
	}

	sigs := target.ExtractSignatures("App.kt", kotlinSample)
	if !strings.Contains(sigs, "data class User") {
		t.Errorf("signatures missing data class User: %s", sigs)
	}
	if !strings.Contains(sigs, "val id: String") {
		t.Errorf("signatures should preserve data class property definitions: %s", sigs)
	}
	if !strings.Contains(sigs, "fun getUser(id: String): User { /* ... */ }") {
		t.Errorf("signatures should collapse getUser body: %s", sigs)
	}
	if strings.Contains(sigs, "Not found") {
		t.Errorf("signatures should not contain function body: %s", sigs)
	}
}

const javaSwitchSample = `package sample;

public class Calc {
    public int eval(String op) {
        switch (op) {
            case "add":
                return 1 + 1;
            case "sub":
                return 1 - 1;
            default:
                return 0;
        }
    }
}
`

const kotlinWhenSample = `package sample

fun eval(op: String): Int {
    return when (op) {
        "add" -> 1 + 1
        "sub" -> {
            1 - 1
        }
        else -> 0
    }
}
`

func TestExtractJVMCase(t *testing.T) {
	// 1. Java switch-case
	body, start, end, err := ExtractJVMCase("Calc.java", javaSwitchSample, "eval", "sub")
	if err != nil {
		t.Fatalf("ExtractJVMCase Java failed: %v", err)
	}
	if !strings.Contains(body, "return 1 - 1;") || strings.Contains(body, "return 1 + 1;") {
		t.Errorf("unexpected Java case body:\n%s", body)
	}
	if start != 8 || end != 9 {
		t.Errorf("Java case lines = %d-%d, want 8-9", start, end)
	}

	// 2. Kotlin when
	bodyKt, startKt, endKt, err := ExtractJVMCase("Calc.kt", kotlinWhenSample, "eval", "sub")
	if err != nil {
		t.Fatalf("ExtractJVMCase Kotlin failed: %v", err)
	}
	if !strings.Contains(bodyKt, "1 - 1") || strings.Contains(bodyKt, "1 + 1") {
		t.Errorf("unexpected Kotlin when body:\n%s", bodyKt)
	}
	if startKt != 6 || endKt != 8 {
		t.Errorf("Kotlin case lines = %d-%d, want 6-8", startKt, endKt)
	}
}
