import { Events } from "@wailsio/runtime";
import {
  ServerService,
  type Branding,
  type Overview,
  type UpdateProgress,
} from "../../bindings/github.com/diablo2org/launcher/internal/app";
import type {
  Choices,
  Ladder,
  NewsItem,
  ServerInfo,
  SettingValue,
  Status,
} from "../../bindings/github.com/diablo2org/launcher/internal/core";
import { errorText } from "./format";

export type Page = "launch" | "ladder" | "catalog" | "checker";

// The last server shown is a convenience for this machine only, so it lives
// in the webview's storage, which can be unavailable.
const lastServerKey = "launcher.lastServer";

function remembered(): string {
  try {
    return localStorage.getItem(lastServerKey) ?? "";
  } catch {
    return "";
  }
}

function remember(id: string) {
  try {
    localStorage.setItem(lastServerKey, id);
  } catch {
    // Not remembering is fine.
  }
}

// Everything the launcher shows for one server. Kept per server so switching
// between pinned servers shows the last known state at once and refreshes
// in the background.
export class ServerData {
  branding = $state<Branding>({ logo: "", background: "" });
  status = $state<Status | null>(null);
  news = $state<NewsItem[] | null>(null);
  ladder = $state<Ladder | null>(null);
  choices = $state<Choices | null>(null);
  settings = $state<SettingValue[]>([]);

  checking = $state(false);
  busy = $state<"" | "updating" | "launching">("");
  progress = $state<{ done: number; total: number; file: string } | null>(null);
  error = $state("");
  newsError = $state("");
  ladderError = $state("");
  copyBytes = $state(0);
  launched = $state(false);

  constructor(public id: string) {}

  async refresh() {
    this.checking = true;
    try {
      const [status, choices, settings] = await Promise.all([
        ServerService.Status(this.id),
        ServerService.Choices(this.id),
        ServerService.Settings(this.id),
      ]);
      this.status = status;
      this.choices = choices;
      this.settings = settings ?? [];
    } catch (err) {
      this.error = errorText(err);
    } finally {
      this.checking = false;
    }
  }

  async loadBranding() {
    this.branding = await ServerService.Branding(this.id);
  }

  async loadNews() {
    try {
      this.news = (await ServerService.News(this.id)) ?? [];
      this.newsError = "";
    } catch (err) {
      this.newsError = errorText(err);
    }
  }

  async loadLadder() {
    try {
      this.ladder = await ServerService.Ladder(this.id);
      this.ladderError = "";
    } catch (err) {
      this.ladderError = errorText(err);
    }
  }

  async update(allowCopy = false) {
    this.error = "";
    this.copyBytes = 0;
    this.busy = "updating";
    this.progress = { done: 0, total: this.status?.updateBytes ?? 0, file: "" };
    try {
      const result = await ServerService.Update(this.id, allowCopy);
      if (result.needsCopy) this.copyBytes = result.copyBytes;
      else if (!result.ok) this.error = result.error;
    } finally {
      this.busy = "";
      this.progress = null;
      await this.refresh();
    }
  }

  async play() {
    this.error = "";
    this.busy = "launching";
    try {
      await ServerService.Play(this.id);
      this.launched = true;
      setTimeout(() => (this.launched = false), 5000);
    } catch (err) {
      this.error = errorText(err);
    } finally {
      this.busy = "";
    }
  }
}

class AppState {
  overview = $state<Overview | null>(null);
  selected = $state("");
  page = $state<Page>("launch");
  settingsOpen = $state(false);
  version = $state("");
  update = $state<{ version: string; url: string } | null>(null);
  error = $state("");

  private data = new Map<string, ServerData>();

  constructor() {
    Events.On("update:progress", (ev: { data: UpdateProgress }) => {
      const p = ev.data.progress;
      this.server(ev.data.server).progress = { done: p.done, total: p.total, file: p.file };
    });
  }

  get servers(): ServerInfo[] {
    return this.overview?.servers ?? [];
  }

  get favourites(): string[] {
    return this.overview?.favourites ?? [];
  }

  get current(): ServerInfo | undefined {
    return this.servers.find((s) => s.id === this.selected);
  }

  get baseReady(): boolean {
    const o = this.overview;
    return !!o && o.base !== "" && !o.baseError && (o.report?.missing ?? []).length === 0;
  }

  info(id: string): ServerInfo | undefined {
    return this.servers.find((s) => s.id === id);
  }

  server(id: string): ServerData {
    let d = this.data.get(id);
    if (!d) {
      d = new ServerData(id);
      this.data.set(id, d);
    }
    return d;
  }

  async load() {
    try {
      const [overview, version] = await Promise.all([ServerService.Overview(), ServerService.Version()]);
      this.overview = overview;
      this.version = version;
      this.error = "";
    } catch (err) {
      this.error = errorText(err);
      return;
    }

    if (this.selected === "" || !this.info(this.selected)) {
      // Reopen on the server used last, else the first pinned one.
      const last = remembered();
      const first = last && this.info(last)?.profile ? last : this.favourites[0];
      if (first) {
        this.select(first);
      } else {
        this.page = "catalog";
      }
    }

    // Warm every pinned server so switching is instant.
    for (const id of this.favourites) this.warm(id);

    ServerService.CheckForUpdate().then((r) => (this.update = r ?? null));
  }

  private warmed = new Set<string>();

  warm(id: string) {
    if (this.warmed.has(id) || !this.info(id)?.profile) return;
    this.warmed.add(id);

    const d = this.server(id);
    d.loadBranding();
    d.loadNews();
    d.refresh();
  }

  select(id: string) {
    this.selected = id;
    remember(id);
    if (this.page === "catalog" || this.page === "checker") this.page = "launch";

    const d = this.server(id);
    if (!this.warmed.has(id)) {
      this.warm(id);
    } else {
      // Already shown once: refresh quietly behind the cached view.
      d.refresh();
    }
  }

  // Switch to the nth pinned server, for Ctrl+1..3.
  selectFavourite(n: number) {
    const id = this.favourites[n];
    if (id) this.select(id);
  }

  async setFavourite(id: string, on: boolean) {
    try {
      await ServerService.SetFavourite(id, on);
      this.error = "";
    } catch (err) {
      this.error = errorText(err);
    }
    this.overview = await ServerService.Overview();
    if (on) this.warm(id);
  }

  // Moves a pinned server from one slot to another. The rail updates straight
  // away; the new order is then saved, and put back if saving fails.
  async moveFavourite(from: number, to: number) {
    const o = this.overview;
    if (!o || from === to) return;

    const before = [...(o.favourites ?? [])];
    const after = [...before];
    const [moved] = after.splice(from, 1);
    after.splice(to, 0, moved);
    o.favourites = after;

    try {
      await ServerService.SetFavouriteOrder(after);
      this.error = "";
    } catch (err) {
      o.favourites = before;
      this.error = errorText(err);
    }
  }

  // Adds a server by URL; returns an error message, or "" on success.
  async addServer(profileURL: string): Promise<string> {
    try {
      const id = await ServerService.AddServer(profileURL);
      this.overview = await ServerService.Overview();
      this.select(id);
      return "";
    } catch (err) {
      return errorText(err);
    }
  }

  async removeServer(id: string) {
    try {
      await ServerService.RemoveServer(id);
      this.error = "";
    } catch (err) {
      this.error = errorText(err);
    }
    this.overview = await ServerService.Overview();
    if (this.selected === id) {
      this.selected = "";
      this.page = "catalog";
    }
  }

  async chooseBase() {
    try {
      this.overview = await ServerService.ChooseBase();
      this.error = "";
      for (const id of this.favourites) this.server(id).refresh();
    } catch (err) {
      this.error = errorText(err);
    }
  }
}

export const app = new AppState();
