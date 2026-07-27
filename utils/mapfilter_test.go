package utils

import (
	"strconv"
	"strings"
	"testing"
)

func TestMap_Empty(t *testing.T) {
	input := []int{}
	expected := []int{}

	result := Map(input, func(i int) string {
		return strconv.Itoa(i)
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
}

func TestMap_IntToString(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	expected := []string{"1", "2", "3", "4", "5"}

	result := Map(input, func(i int) string {
		return strconv.Itoa(i)
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d, expected %s, got %s", i, expected[i], v)
		}
	}
}

func TestMap_StringToUpperCase(t *testing.T) {
	input := []string{"a", "b", "c"}
	expected := []string{"A", "B", "C"}

	result := Map(input, func(s string) string {
		return strings.ToUpper(s)
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d, expected %s, got %s", i, expected[i], v)
		}
	}
}

func FuzzMap_IntToString(f *testing.F) {
	f.Add(123)
	f.Add(0)
	f.Add(-456)

	f.Fuzz(func(t *testing.T, i int) {
		input := []int{i, i * 2, i * 3}
		result := Map(input, func(num int) string {
			return strconv.Itoa(num)
		})

		if len(result) != len(input) {
			t.Fatalf("expected length %d, got %d", len(input), len(result))
		}

		for idx, num := range input {
			expected := strconv.Itoa(num)
			if result[idx] != expected {
				t.Errorf("at index %d, expected %s, got %s", idx, expected, result[idx])
			}
		}
	})
}

func TestFilter_Empty(t *testing.T) {
	input := []int{}
	expected := []int{}

	result := Filter(input, func(n int) bool {
		return n%2 == 0
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
}

func TestFilter_EvenNumbers(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6}
	expected := []int{2, 4, 6}

	result := Filter(input, func(n int) bool {
		return n%2 == 0
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d, expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestFilter_NonEmptyStrings(t *testing.T) {
	input := []string{"hello", "", "world", "", "!"}
	expected := []string{"hello", "world", "!"}

	result := Filter(input, func(s string) bool {
		return s != ""
	})

	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d, expected %s, got %s", i, expected[i], v)
		}
	}
}

func FuzzFilter_PositiveNumbers(f *testing.F) {
	f.Add(-10)
	f.Add(0)
	f.Add(42)

	f.Fuzz(func(t *testing.T, n int) {
		input := []int{n, n + 1, n - 1}
		result := Filter(input, func(num int) bool {
			return num > 0
		})

		for _, v := range result {
			if v <= 0 {
				t.Errorf("expected positive number, got %d", v)
			}
		}
	})
}
