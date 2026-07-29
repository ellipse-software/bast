import defaultMdxComponents from "fumadocs-ui/mdx";
import type { MDXComponents } from "mdx/types";

import { DemoScreenshot } from "@/components/demo-screenshot";

export function getMDXComponents(components?: MDXComponents) {
  return {
    ...defaultMdxComponents,
    DemoScreenshot,
    ...components,
  } satisfies MDXComponents;
}

export const useMDXComponents = getMDXComponents;

declare global {
  type MDXProvidedComponents = ReturnType<typeof getMDXComponents>;
}
