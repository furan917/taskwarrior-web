(function () {
	// Inactive classes are consistent across all tabs.
	// Active classes are per-column (stored in data-active-cls on each button)
	// so they match the column header colours.
	var CLS_INACTIVE = ['border', 'border-zinc-400', 'text-zinc-600', 'font-medium',
	                    'dark:border-zinc-600', 'dark:text-zinc-300'];

	function initKanban() {
		var tabs  = document.getElementById('kanban-tabs');
		var board = document.getElementById('kanban-board');
		if (!tabs || !board) return;

		var mq = window.matchMedia('(min-width: 768px)');
		if (mq.matches) return;

		function activeCls(btn) {
			return (btn.dataset.activeCls || '').split(' ').filter(Boolean);
		}

		function show(colId) {
			board.querySelectorAll('[data-col-id]').forEach(function (col) {
				col.classList.toggle('hidden', col.dataset.colId !== colId);
				col.classList.toggle('block',  col.dataset.colId === colId);
			});
			tabs.querySelectorAll('button[data-col]').forEach(function (btn) {
				var isActive = btn.dataset.col === colId;
				if (isActive) {
					CLS_INACTIVE.forEach(function (c) { btn.classList.remove(c); });
					activeCls(btn).forEach(function (c) { btn.classList.add(c); });
					btn.classList.add('font-semibold');
				} else {
					activeCls(btn).forEach(function (c) { btn.classList.remove(c); });
					btn.classList.remove('font-semibold');
					CLS_INACTIVE.forEach(function (c) { btn.classList.add(c); });
				}
			});
		}

		tabs.addEventListener('click', function (e) {
			var btn = e.target.closest('button[data-col]');
			if (btn) show(btn.dataset.col);
		});

		show('inbox');
	}

	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', initKanban);
	} else {
		initKanban();
	}
})();
