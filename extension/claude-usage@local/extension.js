// Thin loader. GNOME caches extension modules until the next login, so the
// real code lives in main.js and is imported with a unique query string:
// disable + enable then picks up edits without logging out.
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

export default class ClaudeUsageLoader extends Extension {
    enable() {
        this._gen = (this._gen ?? 0) + 1;
        const gen = this._gen;
        import(`file://${this.path}/main.js?v=${Date.now()}`).then(m => {
            if (gen !== this._gen) return; // disabled while loading
            this._impl = new m.default();
            this._impl.enable();
        }).catch(e => console.error(`claude-usage: ${e}\n${e.stack}`));
    }

    disable() {
        this._gen = (this._gen ?? 0) + 1;
        this._impl?.disable();
        this._impl = null;
    }
}
