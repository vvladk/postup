// ─── Audio helpers ────────────────────────────────────────────────────────────

function _playBeep(frequency, duration, volume) {
    try {
        const ctx  = new (window.AudioContext || window.webkitAudioContext)();
        const osc  = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.frequency.value = frequency;
        gain.gain.value     = volume;
        osc.start();
        osc.stop(ctx.currentTime + duration);
        osc.onended = function () { ctx.close(); };
    } catch (_) {}
}

// ─── RetroTimer ───────────────────────────────────────────────────────────────

class RetroTimer {
    constructor(displayEl, inputEl) {
        this.displayEl = displayEl;
        this.inputEl   = inputEl;
        this.interval  = null;
        this.remaining = 0;
        this.initial   = 0;
        this.onFinish  = null;
        this._refreshDisplay();
    }

    _minutesFromInput() {
        return Math.max(1, Math.min(99, parseInt(this.inputEl.value, 10) || 10));
    }

    _setDisplay(seconds) {
        const mm = String(Math.floor(seconds / 60)).padStart(2, '0');
        const ss = String(seconds % 60).padStart(2, '0');
        this.displayEl.textContent = mm + ':' + ss;
    }

    _refreshDisplay() {
        const secs = this.remaining > 0 ? this.remaining : this._minutesFromInput() * 60;
        this._setDisplay(secs);
    }

    start() {
        if (this.interval) return;
        if (this.remaining === 0) {
            this.initial   = this._minutesFromInput() * 60;
            this.remaining = this.initial;
        }
        this.displayEl.classList.remove('timer-display--warning', 'timer-display--finished');
        this.inputEl.disabled = true;
        this.interval = setInterval(function () { this.onTick(); }.bind(this), 1000);
    }

    pause() {
        if (!this.interval) return;
        clearInterval(this.interval);
        this.interval = null;
    }

    reset() {
        clearInterval(this.interval);
        this.interval  = null;
        this.remaining = 0;
        this.initial   = 0;
        this.inputEl.disabled = false;
        this.displayEl.classList.remove('timer-display--warning', 'timer-display--finished');
        this._refreshDisplay();
    }

    onTick() {
        this.remaining--;
        this._setDisplay(this.remaining);

        if (this.remaining <= 0) {
            clearInterval(this.interval);
            this.interval = null;
            this.inputEl.disabled = false;
            this.displayEl.classList.add('timer-display--finished');
            this.playFinalBeep();
            if (this.onFinish) this.onFinish();
            return;
        }

        if (this.remaining <= 30) {
            this.displayEl.classList.add('timer-display--warning');
            if (this.remaining % 5 === 0) {
                this.playWarningBeep();
            }
        }
    }

    playWarningBeep() {
        _playBeep(440, 0.2, 0.3);
    }

    playFinalBeep() {
        _playBeep(880, 1.0, 0.5);
    }
}

// ─── YouTube player ───────────────────────────────────────────────────────────

function extractYouTubeID(url) {
    var patterns = [
        /youtu\.be\/([A-Za-z0-9_-]{11})/,
        /[?&]v=([A-Za-z0-9_-]{11})/,
        /\/embed\/([A-Za-z0-9_-]{11})/,
    ];
    for (var i = 0; i < patterns.length; i++) {
        var m = url.match(patterns[i]);
        if (m) return m[1];
    }
    return null;
}

function loadYouTubePlayer(url, iframeEl, containerEl) {
    var id = extractYouTubeID(url);
    if (!id) {
        alert('Не вдалося розпізнати YouTube посилання');
        return;
    }
    iframeEl.src = 'https://www.youtube.com/embed/' + id + '?autoplay=1';
    containerEl.hidden = false;
}

// ─── Init ─────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', function () {
    var displayEl = document.getElementById('timer-display');
    var inputEl   = document.getElementById('timer-minutes');
    var timer     = new RetroTimer(displayEl, inputEl);

    var btnStart = document.getElementById('btn-start');
    var btnPause = document.getElementById('btn-pause');
    var btnReset = document.getElementById('btn-reset');

    function setRunning(running) {
        btnStart.disabled = running;
        btnPause.disabled = !running;
    }

    timer.onFinish = function () { setRunning(false); };

    btnStart.addEventListener('click', function () {
        timer.start();
        setRunning(true);
    });

    btnPause.addEventListener('click', function () {
        timer.pause();
        setRunning(false);
    });

    btnReset.addEventListener('click', function () {
        timer.reset();
        setRunning(false);
    });

    // Live display update when user changes minutes input before starting
    inputEl.addEventListener('input', function () {
        if (!timer.interval && timer.remaining === 0) {
            timer._refreshDisplay();
        }
    });

    // YouTube
    var btnLoadYT   = document.getElementById('btn-load-yt');
    var ytURL       = document.getElementById('youtube-url');
    var ytIframe    = document.getElementById('youtube-iframe');
    var ytPlayer    = document.getElementById('youtube-player');

    btnLoadYT.addEventListener('click', function () {
        loadYouTubePlayer(ytURL.value.trim(), ytIframe, ytPlayer);
    });

    ytURL.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') loadYouTubePlayer(ytURL.value.trim(), ytIframe, ytPlayer);
    });
});
