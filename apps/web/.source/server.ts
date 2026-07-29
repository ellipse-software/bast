// @ts-nocheck
import * as __fd_glob_26 from "../content/docs/features/(sync)/gcp.mdx?collection=docs"
import * as __fd_glob_25 from "../content/docs/features/(sync)/azure.mdx?collection=docs"
import * as __fd_glob_24 from "../content/docs/features/(sync)/aws.mdx?collection=docs"
import * as __fd_glob_23 from "../content/docs/features/(other)/shortcuts.mdx?collection=docs"
import * as __fd_glob_22 from "../content/docs/features/(other)/mobile-view.mdx?collection=docs"
import * as __fd_glob_21 from "../content/docs/features/(other)/cli.mdx?collection=docs"
import * as __fd_glob_20 from "../content/docs/features/(keys)/keys.mdx?collection=docs"
import * as __fd_glob_19 from "../content/docs/features/(keys)/key-selector.mdx?collection=docs"
import * as __fd_glob_18 from "../content/docs/features/(hosts)/ssh-config.mdx?collection=docs"
import * as __fd_glob_17 from "../content/docs/features/(hosts)/host-picker.mdx?collection=docs"
import * as __fd_glob_16 from "../content/docs/features/(hosts)/host-management.mdx?collection=docs"
import * as __fd_glob_15 from "../content/docs/features/(hosts)/history-import.mdx?collection=docs"
import * as __fd_glob_14 from "../content/docs/features/(hosts)/connections.mdx?collection=docs"
import * as __fd_glob_13 from "../content/docs/features/(hosts)/advanced-host-options.mdx?collection=docs"
import * as __fd_glob_12 from "../content/docs/reference/updates.mdx?collection=docs"
import * as __fd_glob_11 from "../content/docs/reference/telemetry.mdx?collection=docs"
import * as __fd_glob_10 from "../content/docs/reference/install.mdx?collection=docs"
import * as __fd_glob_9 from "../content/docs/reference/files.mdx?collection=docs"
import * as __fd_glob_8 from "../content/docs/reference/agents.mdx?collection=docs"
import * as __fd_glob_7 from "../content/docs/index.mdx?collection=docs"
import { default as __fd_glob_6 } from "../content/docs/features/(keys)/meta.json?collection=docs"
import { default as __fd_glob_5 } from "../content/docs/features/(sync)/meta.json?collection=docs"
import { default as __fd_glob_4 } from "../content/docs/features/(other)/meta.json?collection=docs"
import { default as __fd_glob_3 } from "../content/docs/features/(hosts)/meta.json?collection=docs"
import { default as __fd_glob_2 } from "../content/docs/reference/meta.json?collection=docs"
import { default as __fd_glob_1 } from "../content/docs/features/meta.json?collection=docs"
import { default as __fd_glob_0 } from "../content/docs/meta.json?collection=docs"
import { server } from 'fumadocs-mdx/runtime/server';
import type * as Config from '../source.config';

const create = server<typeof Config, import("fumadocs-mdx/runtime/types").InternalTypeConfig & {
  DocData: {
  }
}>();

export const docs = await create.docs("docs", "content/docs", {"meta.json": __fd_glob_0, "features/meta.json": __fd_glob_1, "reference/meta.json": __fd_glob_2, "features/(hosts)/meta.json": __fd_glob_3, "features/(other)/meta.json": __fd_glob_4, "features/(sync)/meta.json": __fd_glob_5, "features/(keys)/meta.json": __fd_glob_6, }, {"index.mdx": __fd_glob_7, "reference/agents.mdx": __fd_glob_8, "reference/files.mdx": __fd_glob_9, "reference/install.mdx": __fd_glob_10, "reference/telemetry.mdx": __fd_glob_11, "reference/updates.mdx": __fd_glob_12, "features/(hosts)/advanced-host-options.mdx": __fd_glob_13, "features/(hosts)/connections.mdx": __fd_glob_14, "features/(hosts)/history-import.mdx": __fd_glob_15, "features/(hosts)/host-management.mdx": __fd_glob_16, "features/(hosts)/host-picker.mdx": __fd_glob_17, "features/(hosts)/ssh-config.mdx": __fd_glob_18, "features/(keys)/key-selector.mdx": __fd_glob_19, "features/(keys)/keys.mdx": __fd_glob_20, "features/(other)/cli.mdx": __fd_glob_21, "features/(other)/mobile-view.mdx": __fd_glob_22, "features/(other)/shortcuts.mdx": __fd_glob_23, "features/(sync)/aws.mdx": __fd_glob_24, "features/(sync)/azure.mdx": __fd_glob_25, "features/(sync)/gcp.mdx": __fd_glob_26, });