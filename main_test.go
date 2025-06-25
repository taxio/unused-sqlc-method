package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetStructMethods(t *testing.T) {
	testDir := createTestFiles(t)
	defer os.RemoveAll(testDir)

	tests := []struct {
		name       string
		structName string
		want       []string
	}{
		{
			name:       "simple struct with methods",
			structName: "TestStruct",
			want:       []string{"Method1", "Method2"},
		},
		{
			name:       "struct with pointer receiver",
			structName: "PointerStruct",
			want:       []string{"PointerMethod"},
		},
		{
			name:       "non-existent struct",
			structName: "NonExistent",
			want:       []string{},
		},
		{
			name:       "struct methods across multiple files",
			structName: "MultiFileStruct",
			want:       []string{"FileOneMethod", "FileTwoMethod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getStructMethods(testDir, tt.structName)
			if err != nil {
				t.Errorf("getStructMethods() error = %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("getStructMethods() = %v, want %v", got, tt.want)
				return
			}

			for _, method := range got {
				found := false
				for _, wantMethod := range tt.want {
					if method.MethodName == wantMethod {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getStructMethods() = %v, want %v", got, tt.want)
					return
				}
			}
		})
	}
}

func createTestFiles(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "test-struct-methods")
	if err != nil {
		t.Fatal(err)
	}

	testFileContent := `package test

type TestStruct struct {
	field string
}

func (t TestStruct) Method1() string {
	return t.field
}

func (t *TestStruct) Method2() {
	t.field = "updated"
}

type PointerStruct struct {
	value int
}

func (p *PointerStruct) PointerMethod() int {
	return p.value
}

type AnotherStruct struct {
	data []byte
}

func (a AnotherStruct) AnotherMethod() {
	// do nothing
}
`

	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte(testFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create additional files to test multi-file detection
	file1Content := `package test

type MultiFileStruct struct {
	data string
}

func (m *MultiFileStruct) FileOneMethod() {
	m.data = "file1"
}
`
	file1 := filepath.Join(tmpDir, "file1.go")
	if err := os.WriteFile(file1, []byte(file1Content), 0644); err != nil {
		t.Fatal(err)
	}

	file2Content := `package test

func (m MultiFileStruct) FileTwoMethod() string {
	return m.data
}
`
	file2 := filepath.Join(tmpDir, "file2.go")
	if err := os.WriteFile(file2, []byte(file2Content), 0644); err != nil {
		t.Fatal(err)
	}

	return tmpDir
}

func TestFindUsedMethods(t *testing.T) {
	testDir := createUsageTestFiles(t)
	defer os.RemoveAll(testDir)

	methods := []StructMethod{
		{PackageName: "main", ReceiverName: "TestStruct", MethodName: "UsedMethod"},
		{PackageName: "main", ReceiverName: "TestStruct", MethodName: "UnusedMethod"},
		{PackageName: "main", ReceiverName: "TestStruct", MethodName: "AnotherMethod"},
	}
	
	usedMethods, err := findUsedMethods(testDir, "TestStruct", methods)
	if err != nil {
		t.Errorf("findUsedMethods() error = %v", err)
		return
	}

	expected := []string{"UsedMethod", "AnotherMethod"}
	if len(usedMethods) != len(expected) {
		t.Errorf("findUsedMethods() = %v, want %v", usedMethods, expected)
		return
	}

	for _, expectedMethod := range expected {
		found := false
		for _, usedMethod := range usedMethods {
			if expectedMethod == usedMethod.MethodName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected method %s not found in used methods: %v", expectedMethod, usedMethods)
		}
	}
}

func createUsageTestFiles(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "test-usage")
	if err != nil {
		t.Fatal(err)
	}

	// File with method usage
	usageFileContent := `package main

import "fmt"

func main() {
	var ts TestStruct
	ts.UsedMethod()
	
	result := ts.AnotherMethod()
	fmt.Println(result)
}
`

	usageFile := filepath.Join(tmpDir, "usage.go")
	if err := os.WriteFile(usageFile, []byte(usageFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Another file without method usage
	anotherFileContent := `package main

func someOtherFunction() {
	fmt.Println("This file doesn't use TestStruct methods")
}
`

	anotherFile := filepath.Join(tmpDir, "other.go")
	if err := os.WriteFile(anotherFile, []byte(anotherFileContent), 0644); err != nil {
		t.Fatal(err)
	}

	return tmpDir
}