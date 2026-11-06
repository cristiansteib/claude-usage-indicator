import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import St from 'gi://St';
import Clutter from 'gi://Clutter';
import Gio from 'gi://Gio';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PanelMenu from 'resource:///org/gnome/shell/ui/panelMenu.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';

// unique per load, so hot-reloading never re-registers an existing GType name
const TAG = (import.meta.url.split('?v=')[1] ?? '0').replace(/\W/g, '');
const BAR_W = 200;
const BAR_H = 7;
const RING = 30;
const LOGO = 24;
const TAU = Math.PI * 2;

function hex(h, a = 1) {
    return [parseInt(h.slice(1, 3), 16) / 255, parseInt(h.slice(3, 5), 16) / 255, parseInt(h.slice(5, 7), 16) / 255, a];
}

function color(pct) {
    if (pct >= 85) return '#f24b4b';
    if (pct >= 60) return '#f5a623';
    return '#4cd964';
}

function clock(unix) {
    return unix ? GLib.DateTime.new_from_unix_local(unix).format('%H:%M') : '?';
}

function when(unix) {
    return unix ? GLib.DateTime.new_from_unix_local(unix).format('%a %H:%M') : '?';
}

function ago(unix) {
    if (!unix) return 'never';
    const s = Math.max(0, Math.floor(Date.now() / 1000) - unix);
    if (s < 60) return 'just now';
    if (s < 3600) return `${Math.floor(s / 60)} min ago`;
    return `${Math.floor(s / 3600)} h ${Math.floor((s % 3600) / 60)} min ago`;
}

// Pace ring: one lap = today's cumulative budget (day N of 7 x 14.3%).
// Going past 100% starts a second lap in a different color (overlap).
// A translucent "ghost" arc shows where the current session is forecast to take it.
const Ring = GObject.registerClass({GTypeName: `ClaudeRing${TAG}`},
class Ring extends St.DrawingArea {
    _init() {
        super._init({width: RING, height: RING, y_align: Clutter.ActorAlign.CENTER});
        this._frac = 0;
        this._proj = 0;
        this.connect('repaint', () => this._draw());
    }

    set(frac, proj) {
        this._frac = Math.max(0, frac || 0);
        this._proj = Math.max(this._frac, proj || 0);
        this.queue_repaint();
    }

    _arc(cr, c, r, lap, rgba) {
        if (lap <= 0) return;
        cr.setSourceRGBA(...rgba);
        if (lap >= 0.999) cr.arc(c, c, r, 0, TAU);
        else cr.arc(c, c, r, -Math.PI / 2, -Math.PI / 2 + lap * TAU);
        cr.stroke();
    }

    _laps(cr, c, r, v, a) {
        if (v > 1) {
            this._arc(cr, c, r, 1, hex('#e0443e', a));                    // first lap, full
            this._arc(cr, c, r, Math.min(v - 1, 1), hex('#ffb3d1', a));   // overlap lap on top
        } else {
            this._arc(cr, c, r, v, hex(v >= 0.85 ? '#f5a623' : '#4cd964', a));
        }
    }

    _draw() {
        const cr = this.get_context();
        const [w] = this.get_surface_size();
        const c = w / 2, lw = 4.5, r = c - lw / 2 - 0.5;
        const f = this._frac;
        cr.setLineWidth(lw);
        cr.setLineCap(1); // round
        this._arc(cr, c, r, 1, [1, 1, 1, 0.22]);
        if (this._proj > f + 0.005)
            this._laps(cr, c, r, this._proj, 0.38);
        this._laps(cr, c, r, f, 1);
        // center: % of today's budget
        const txt = `${Math.round(f * 100)}`;
        cr.setSourceRGBA(1, 1, 1, 1);
        cr.selectFontFace('Sans', 0, 1);
        cr.setFontSize(txt.length > 2 ? 8 : 9.5);
        const e = cr.textExtents(txt);
        cr.moveTo(c - (e.width / 2 + e.xBearing), c - (e.height / 2 + e.yBearing));
        cr.showText(txt);
        cr.$dispose();
    }
});

// Claude-style starburst, drawn with Cairo so no asset files are needed.
const Logo = GObject.registerClass({GTypeName: `ClaudeLogo${TAG}`},
class Logo extends St.DrawingArea {
    _init() {
        super._init({width: LOGO, height: LOGO, y_align: Clutter.ActorAlign.CENTER});
        this.connect('repaint', () => this._draw());
    }

    _draw() {
        const cr = this.get_context();
        const [w] = this.get_surface_size();
        const c = w / 2;
        const len = [1, 0.78, 0.95, 0.72, 1, 0.8, 0.92, 0.74, 1, 0.78, 0.95, 0.72];
        cr.setSourceRGBA(...hex('#d97757'));
        cr.setLineWidth(2.4);
        cr.setLineCap(1);
        for (let i = 0; i < 12; i++) {
            const a = i * TAU / 12 - Math.PI / 2;
            cr.moveTo(c + Math.cos(a) * c * 0.2, c + Math.sin(a) * c * 0.2);
            cr.lineTo(c + Math.cos(a) * (c - 1.5) * len[i], c + Math.sin(a) * (c - 1.5) * len[i]);
            cr.stroke();
        }
        cr.$dispose();
    }
});

const Bar = GObject.registerClass({GTypeName: `ClaudeBar${TAG}`},
class Bar extends St.BoxLayout {
    _init() {
        super._init({
            style: `width:${BAR_W}px; height:${BAR_H}px; border-radius:3px; background-color:rgba(255,255,255,0.25);`,
        });
        this._fill = new St.Widget({style: `width:0px; height:${BAR_H}px; border-radius:3px;`});
        this.add_child(this._fill);
    }

    setPct(pct) {
        const w = Math.round(Math.max(0, Math.min(100, pct)) / 100 * BAR_W);
        this._fill.set_style(`width:${w}px; height:${BAR_H}px; border-radius:3px; background-color:${color(pct)};`);
    }
});

const Indicator = GObject.registerClass({GTypeName: `ClaudeIndicator${TAG}`},
class Indicator extends PanelMenu.Button {
    _init() {
        super._init(0.5, 'Claude usage', false);

        const row = new St.BoxLayout({style: 'spacing:9px;', y_align: Clutter.ActorAlign.CENTER});
        this._ring = new Ring();
        const bars = new St.BoxLayout({vertical: true, y_align: Clutter.ActorAlign.CENTER, style: 'spacing:4px;'});
        this._session = new Bar();
        this._weekly = new Bar();
        bars.add_child(this._session);
        bars.add_child(this._weekly);
        row.add_child(this._ring);   // left: pace ring
        row.add_child(bars);         // middle: session + weekly
        row.add_child(new Logo());   // right: Claude icon
        this.add_child(row);

        const item = (t, style) => {
            const i = new PopupMenu.PopupMenuItem(t, {reactive: false});
            if (style) i.label.set_style(style);
            this.menu.addMenuItem(i);
            return i;
        };
        this._iSession = item('Session (5h): …');
        this._iWeekly = item('Weekly (7d): …');
        this._iToday = item('Today: …');
        this._iForecast = item('Forecast: …');
        this.menu.addMenuItem(new PopupMenu.PopupSeparatorMenuItem());
        this._iUpdated = item('Last update: …');
        this._iStatus = item('', 'font-size:90%;');
        this._iStatus.visible = false;
        this._iRefresh = new PopupMenu.PopupMenuItem('⟳  Refresh now');
        this._iRefresh.connect('activate', () => this._refreshNow());
        this.menu.addMenuItem(this._iRefresh);

        this._file = Gio.File.new_for_path(`${GLib.getenv('XDG_RUNTIME_DIR')}/claude-usage.json`);
        // first read after the shell is idle, so enabling at login costs nothing
        this._idle = GLib.idle_add(GLib.PRIORITY_LOW, () => {
            this._idle = null;
            this._load();
            return GLib.SOURCE_REMOVE;
        });
        this._timer = GLib.timeout_add_seconds(GLib.PRIORITY_LOW, 10, () => {
            this._load();
            return GLib.SOURCE_CONTINUE;
        });
        this.menu.connect('open-state-changed', (_m, open) => open && this._load());
    }

    _refreshNow() {
        this._iStatus.label.text = 'Refreshing…';
        this._iStatus.visible = true;
        Gio.DBus.session.call('local.ClaudeUsage', '/local/ClaudeUsage', 'local.ClaudeUsage', 'Refresh',
            null, new GLib.VariantType('(s)'), Gio.DBusCallFlags.NONE, 25000, null, (conn, res) => {
                try {
                    conn.call_finish(res);
                } catch (e) {
                    this._iStatus.label.text = 'Daemon not running (claude-usage-indicator)';
                    return;
                }
                this._load();
            });
    }

    // Asynchronous read: never blocks GNOME Shell's main thread.
    _load() {
        if (this._reading || this._destroyed) return;
        this._reading = true;
        this._file.load_contents_async(null, (f, res) => {
            this._reading = false;
            if (this._destroyed) return;
            let d;
            try {
                const [ok, bytes] = f.load_contents_finish(res);
                if (!ok) return;
                d = JSON.parse(new TextDecoder().decode(bytes));
            } catch (e) {
                this._iUpdated.label.text = 'Waiting for claude-usage-indicator…';
                return;
            }
            this._apply(d);
        });
    }

    _apply(d) {
        if (d.has_usage) {
            const dl = d.daily, fc = d.forecast;
            this._session.setPct(d.session_pct);
            this._weekly.setPct(d.weekly_pct);
            this._ring.set(dl.frac, fc && fc.ready ? fc.proj_frac : 0);
            this._iSession.label.text = `Session (5h): ${Math.round(d.session_pct)}%  — resets ${when(d.session_reset)}`;
            this._iWeekly.label.text = `Weekly (7d): ${Math.round(d.weekly_pct)}%  — resets ${when(d.weekly_reset)}`;
            const sign = dl.pace >= 0 ? '+' : '−';
            this._iToday.label.text =
                `Day ${dl.day} of 7 · ${dl.per_day.toFixed(1)}% per day\n` +
                `Budget through today: ${dl.budget.toFixed(1)}% · used ${dl.weekly.toFixed(1)}% (${Math.round(dl.frac * 100)}%)` +
                (dl.frac > 1 ? '  — ahead of pace' : '') +
                `\nPace: ${sign}${Math.abs(dl.pace).toFixed(1)}% vs an even burn · ${dl.days_left.toFixed(1)} days left`;
            if (fc && fc.ready) {
                this._iForecast.label.text =
                    `Forecast at session end (${fc.hours_left.toFixed(1)} h): weekly ≈ ${fc.proj_weekly.toFixed(1)}% ` +
                    `(+${fc.add_weekly.toFixed(1)}%)\n` +
                    `Chance of exceeding today's budget: ${Math.round(fc.p_over * 100)}%  ` +
                    `(burn ${fc.rate.toFixed(1)}%/h, 1% session ≈ ${fc.k.toFixed(2)}% week)`;
            } else {
                this._iForecast.label.text = `Forecast: ${fc ? fc.note : '…'}`;
            }
        }
        this._iUpdated.label.text = `Last update: ${d.fetched_at ? when(d.fetched_at) : 'never'} (${ago(d.fetched_at)})`;

        const lines = [];
        if (Date.now() / 1000 - d.updated > 150)
            lines.push('⚠ Daemon not running — start claude-usage-indicator');
        if (d.status === 'stale' || d.status === 'rate_limited')
            lines.push(`⚠ ${d.message}. Showing cached data; next request after ${clock(d.blocked_until)}`);
        else if (d.status === 'error')
            lines.push(`⚠ ${d.message}`);
        else if (d.next_poll)
            lines.push(`Next automatic refresh ≈ ${clock(d.next_poll)}`);
        this._iStatus.label.text = lines.join('\n');
        this._iStatus.visible = lines.length > 0;
    }

    destroy() {
        this._destroyed = true;
        if (this._idle) {
            GLib.source_remove(this._idle);
            this._idle = null;
        }
        if (this._timer) {
            GLib.source_remove(this._timer);
            this._timer = null;
        }
        super.destroy();
    }
});

export default class Impl {
    enable() {
        this._indicator = new Indicator();
        // index 1 in the center box = right next to the date/clock (dateMenu is index 0)
        Main.panel.addToStatusArea('claude-usage', this._indicator, 1, 'center');
    }

    disable() {
        this._indicator?.destroy();
        this._indicator = null;
    }
}
