package loopdetect

import "testing"

func TestBuildFingerprintDeterministic(t *testing.T) {
	first := BuildFingerprint("read_document", "hash-a", "内容")
	second := BuildFingerprint("READ_DOCUMENT ", "hash-a", " 内容 ")
	if first != second {
		t.Fatalf("规范化后指纹应一致: %s vs %s", first, second)
	}
	other := BuildFingerprint("read_document", "hash-b", "内容")
	if first == other {
		t.Fatalf("不同参数哈希应产生不同指纹")
	}
}

func TestDetectThreshold(t *testing.T) {
	fp := BuildFingerprint("read_document", "hash-a", "内容")
	result := Detect(nil, fp, 3)
	if result.Detected || result.Repeats != 1 {
		t.Fatalf("首次出现不应判定循环: %+v", result)
	}
	history := []string{fp}
	result = Detect(history, fp, 3)
	if result.Detected || result.Repeats != 2 {
		t.Fatalf("第二次出现不应判定循环: %+v", result)
	}
	history = Append(history, fp)
	result = Detect(history, fp, 3)
	if !result.Detected || result.Repeats != 3 {
		t.Fatalf("第三次连续出现应判定循环: %+v", result)
	}
}

func TestDetectResetOnDifferentFingerprint(t *testing.T) {
	fp := BuildFingerprint("read_document", "hash-a", "内容")
	other := BuildFingerprint("write_artifact", "hash-b", "别的")
	history := []string{fp, fp, other}
	result := Detect(history, fp, 3)
	if result.Detected {
		t.Fatalf("被其他 step 打断后不应判定循环: %+v", result)
	}
}

func TestAppendCapsHistory(t *testing.T) {
	var history []string
	for i := 0; i < MaxHistory+10; i++ {
		history = Append(history, BuildFingerprint("tool", "hash", string(rune('a'+i%26))))
	}
	if len(history) != MaxHistory {
		t.Fatalf("历史长度应被裁剪到 %d,实际 %d", MaxHistory, len(history))
	}
}
