import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
  MarkdownCopyButton,
  ViewOptionsPopover,
} from "fumadocs-ui/layouts/docs/page";
import { createRelativeLink } from "fumadocs-ui/mdx";
import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { getMDXComponents } from "@/components/mdx";
import { bastWebDocsPath, getLatestBastVersion } from "@/lib/github";
import { getPageMarkdownUrl } from "@/lib/llms";
import { createPageMetadata } from "@/lib/metadata";
import {
  preWindowsInstallDescription,
  supportsWindowsRelease,
} from "@/lib/releases";
import { source } from "@/lib/source";

export default async function Page(props: PageProps<"/docs/[[...slug]]">) {
  const params = await props.params;
  const page = source.getPage(params.slug);
  if (!page) notFound();

  const MDX = page.data.body;
  const markdownUrl = getPageMarkdownUrl(page).url;
  const githubUrl = `${bastWebDocsPath}/${page.path}`;
  const windowsAvailable =
    page.url !== "/docs/install" ||
    supportsWindowsRelease(await getLatestBastVersion());
  const description = windowsAvailable
    ? page.data.description
    : preWindowsInstallDescription;
  const toc = windowsAvailable
    ? page.data.toc
    : page.data.toc.filter(
        ({ url }) => url !== "#windows-11" && url !== "#wsl",
      );

  return (
    <DocsPage toc={toc} full={page.data.full}>
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{description}</DocsDescription>
      <div className="not-prose mb-6 flex flex-row flex-wrap items-center gap-2 border-b border-fd-border pb-6">
        <MarkdownCopyButton markdownUrl={markdownUrl} />
        <ViewOptionsPopover markdownUrl={markdownUrl} githubUrl={githubUrl} />
      </div>
      <DocsBody>
        <MDX
          components={getMDXComponents({
            a: createRelativeLink(source, page),
          })}
        />
      </DocsBody>
    </DocsPage>
  );
}

export function generateStaticParams() {
  return source.generateParams();
}

export async function generateMetadata(
  props: PageProps<"/docs/[[...slug]]">,
): Promise<Metadata> {
  const params = await props.params;
  const page = source.getPage(params.slug);
  if (!page) notFound();
  const windowsAvailable =
    page.url !== "/docs/install" ||
    supportsWindowsRelease(await getLatestBastVersion());

  return createPageMetadata({
    title: page.data.title,
    description: windowsAvailable
      ? page.data.description
      : preWindowsInstallDescription,
    path: page.url,
  });
}
