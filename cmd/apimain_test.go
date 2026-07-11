package main

import "testing"

// TestIsValidDatabaseName 覆盖库名格式校验的合法/非法样例。
func TestIsValidDatabaseName(t *testing.T) {
	valid := []string{
		"rustdesk",
		"rustdesk_api",
		"_internal",
		"db2024",
		"a",
		"Abc123_Xy",
	}
	for _, name := range valid {
		if !isValidDatabaseName(name) {
			t.Errorf("expected %q to be a valid database name", name)
		}
	}

	invalid := []string{
		"",            // 空
		"1db",         // 数字开头
		"db-name",     // 连字符
		"db name",     // 空格
		"db;DROP",     // 注入
		"db`x",        // 反引号
		"../../etc",   // 路径穿越字符
		"db.name",     // 点
		"-db",         // 下划线以外符号开头
	}
	for _, name := range invalid {
		if isValidDatabaseName(name) {
			t.Errorf("expected %q to be an invalid database name", name)
		}
	}

	// 长度超过 64 应判为非法
	tooLong := make([]byte, 65)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if isValidDatabaseName(string(tooLong)) {
		t.Errorf("expected 65-char name to be invalid (length limit 64)")
	}

	// 恰好 64 位应合法
	exact := make([]byte, 64)
	for i := range exact {
		exact[i] = 'a'
	}
	if !isValidDatabaseName(string(exact)) {
		t.Errorf("expected 64-char name to be valid")
	}
}
