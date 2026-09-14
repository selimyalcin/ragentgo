package rag_test

import (
	"context"
	"fmt"

	"github.com/selimyalcin/ragentgo/document"
	"github.com/selimyalcin/ragentgo/rag"
	"github.com/selimyalcin/ragentgo/store/memory"
)

func ExamplePipeline() {
	pipe, err := rag.New(
		rag.WithEmbedder(hashEmbedder{}),
		rag.WithVectorStore(memory.New()),
		rag.WithGenerator(echoGen{}),
	)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	_ = pipe.IndexDocument(ctx, document.Document{
		Content: "RAGentGo is a RAG toolkit written in Go.",
		Source:  "readme.md",
	})

	result, err := pipe.Query(ctx, "What is RAGentGo?", rag.QueryOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(result.Answer) > 0)
	fmt.Println(len(result.Sources) > 0)
	// Output:
	// true
	// true
}
