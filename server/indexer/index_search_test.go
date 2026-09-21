package indexer

import "testing"

func TestIndexSearchGetByNameSizeAllSymbols(t *testing.T) {
	idx := newIndex()
	idx.PutFile("file:///HttpManager.php", []*Symbol{
		{URI: "file:///HttpManager.php", FQN: `\App\HttpManager`, Name: "HttpManager", Kind: KindClass},
		methodSymbol("file:///HttpManager.php", `\App\HttpManager::handle`, "handle"),
	})
	idx.PutFile("file:///Helper.php", []*Symbol{
		{URI: "file:///Helper.php", FQN: `\App\Helper`, Name: "Helper", Kind: KindClass},
	})

	if got := idx.Size(); got != 3 {
		t.Fatalf("Size=%d want 3", got)
	}
	if got := idx.AllSymbols(); len(got) != 3 {
		t.Fatalf("AllSymbols=%d want 3", len(got))
	}

	byName := idx.GetByName("httpmanager")
	if len(byName) != 1 || byName[0].Name != "HttpManager" {
		t.Fatalf("GetByName: %#v", byName)
	}

	search := idx.Search("manag")
	if len(search) != 1 || search[0].Name != "HttpManager" {
		t.Fatalf("Search: %#v", search)
	}

	fuzzy := idx.FuzzySearch("HM")
	if len(fuzzy) != 1 || fuzzy[0].Name != "HttpManager" {
		t.Fatalf("FuzzySearch HM: %#v", fuzzy)
	}
	if !camelMatch("HttpResponseFactory", "HRF") {
		t.Fatal("camelMatch HRF")
	}
	if camelMatch("HttpManager", "ZZ") {
		t.Fatal("camelMatch should reject ZZ")
	}
}

func TestIndexRemoveMissingIsNoop(t *testing.T) {
	idx := newIndex()
	idx.RemoveFile("file:///missing.php")
	if idx.Size() != 0 {
		t.Fatal("expected empty index")
	}
}
