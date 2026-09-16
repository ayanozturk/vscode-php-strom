package indexer

import "testing"

func TestIndexDocumentMergesPromotedAndEnumValue(t *testing.T) {
	wi := New(Config{})
	uri := "file:///workspace/Session.php"
	text := `<?php
class SessionStore {}
class Session {
	public function __construct(private SessionStore $session) {}
}`
	wi.IndexDocument(uri, text)
	found := false
	for _, s := range wi.GetIndex().GetByURI(uri) {
		t.Logf("sym %s kind=%v", s.FQN, s.Kind)
		if s.FQN == `\Session::$session` {
			found = true
		}
	}
	if !found {
		t.Fatal("missing promoted property symbol")
	}

	uri2 := "file:///workspace/InvoiceStatus.php"
	text2 := `<?php
namespace App\Module\Subscription\Enum;
enum InvoiceStatus: int { case PENDING = 1; }
`
	wi.IndexDocument(uri2, text2)
	foundVal := false
	for _, s := range wi.GetIndex().GetByURI(uri2) {
		t.Logf("enum sym %s", s.FQN)
		if s.FQN == `\App\Module\Subscription\Enum\InvoiceStatus::$value` {
			foundVal = true
		}
	}
	if !foundVal {
		t.Fatal("missing backed enum $value")
	}
}
