// Conversion automatique des dates vers le timezone du navigateur
(function() {
    'use strict';

    /**
     * Formate une date ISO 8601 vers le timezone local du navigateur
     * @param {string} isoDate - Date au format ISO 8601 (ex: "2026-02-09T00:19:00Z")
     * @param {string} format - Format de sortie: 'datetime', 'date', 'time'
     * @returns {string} Date formattée
     */
    function formatLocalDate(isoDate, format) {
        if (!isoDate) return '';
        
        const date = new Date(isoDate);
        if (isNaN(date.getTime())) return isoDate; // Date invalide, retourner tel quel

        const options = {
            datetime: {
                day: '2-digit',
                month: '2-digit',
                year: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            },
            date: {
                day: '2-digit',
                month: '2-digit',
                year: 'numeric'
            },
            time: {
                hour: '2-digit',
                minute: '2-digit'
            }
        };

        return date.toLocaleString('fr-FR', options[format] || options.datetime);
    }

    /**
     * Convertit tous les éléments avec l'attribut data-utc-date
     */
    function convertAllDates() {
        const elements = document.querySelectorAll('[data-utc-date]');
        
        elements.forEach(function(el) {
            const utcDate = el.getAttribute('data-utc-date');
            const format = el.getAttribute('data-format') || 'datetime';
            
            if (utcDate) {
                el.textContent = formatLocalDate(utcDate, format);
            }
        });
    }

    // Convertir dès que le DOM est prêt
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', convertAllDates);
    } else {
        convertAllDates();
    }
})();
