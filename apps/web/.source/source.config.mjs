// source.config.ts
import { defineConfig, defineDocs } from "fumadocs-mdx/config";
var llmsOptions = {
  mdxAsPlaceholder: ["DemoScreenshot"]
};
var docs = defineDocs({
  dir: "content/docs",
  docs: {
    postprocess: {
      includeProcessedMarkdown: llmsOptions
    }
  }
});
var source_config_default = defineConfig();
export {
  source_config_default as default,
  docs
};
