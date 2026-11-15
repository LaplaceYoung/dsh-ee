package registry

// npm publish 附件的上限检查：解码前按 base64 编码长度拦掉必然超限的 tarball，
// 避免为一份注定被拒的请求再分配解码后的字节。

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestPublishAttachmentDecodeLimit(t *testing.T) {
	const maxTarball = 1024
	atLimit := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), maxTarball))
	raw, err := decodePublishAttachment(atLimit, maxTarball)
	if err != nil || len(raw) != maxTarball {
		t.Fatalf("上限内的附件应正常解码：len=%d err=%v", len(raw), err)
	}

	// 解码后 1027 字节：编码长度已经超过上限，应该在分配解码结果之前就被拒。
	_, err = decodePublishAttachment(atLimit+"AAAA", maxTarball)
	var re *Error
	if !errors.As(err, &re) || re.Status != 413 {
		t.Fatalf("超过上限的附件应返回 413，得到 %v", err)
	}

	if _, err := decodePublishAttachment("!!!", maxTarball); !errors.As(err, &re) || re.Status != 400 {
		t.Fatalf("非法 base64 应返回 400，得到 %v", err)
	}
}
