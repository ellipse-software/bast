import { defineConfig, defineDocs } from "fumadocs-mdx/config";
import type { LLMsOptions } from "fumadocs-core/mdx-plugins/remark-llms";

const llmsOptions: LLMsOptions = {
  mdxAsPlaceholder: ["DemoScreenshot"],
};

export const docs = defineDocs({
  dir: "content/docs",
  docs: {
    postprocess: {
      includeProcessedMarkdown: llmsOptions,
    },
  },
});

export default defineConfig();
