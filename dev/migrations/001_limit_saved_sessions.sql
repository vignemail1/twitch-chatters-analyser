-- Migration: Limiter le nombre de sessions sauvegardées à 10 par utilisateur
-- Date: 2026-02-09

-- Créer une procédure stockée pour nettoyer les anciennes sessions
DROP PROCEDURE IF EXISTS cleanup_old_saved_sessions;

DELIMITER $$

CREATE PROCEDURE cleanup_old_saved_sessions(IN p_user_id BIGINT UNSIGNED)
BEGIN
    DECLARE v_count INT;
    
    -- Compter le nombre de sessions sauvegardées pour cet utilisateur
    SELECT COUNT(*) INTO v_count
    FROM sessions
    WHERE user_id = p_user_id
      AND status = 'saved';
    
    -- Si plus de 10 sessions, supprimer les plus anciennes
    IF v_count > 10 THEN
        DELETE FROM sessions
        WHERE id IN (
            SELECT id FROM (
                SELECT id
                FROM sessions
                WHERE user_id = p_user_id
                  AND status = 'saved'
                ORDER BY updated_at ASC
                LIMIT (v_count - 10)
            ) AS old_sessions
        );
    END IF;
END$$

DELIMITER ;

-- Créer un trigger qui s'exécute après chaque UPDATE de status vers 'saved'
DROP TRIGGER IF EXISTS after_session_saved;

DELIMITER $$

CREATE TRIGGER after_session_saved
AFTER UPDATE ON sessions
FOR EACH ROW
BEGIN
    -- Si le status vient de passer à 'saved', nettoyer les anciennes sessions
    IF NEW.status = 'saved' AND OLD.status != 'saved' THEN
        CALL cleanup_old_saved_sessions(NEW.user_id);
    END IF;
END$$

DELIMITER ;

-- Appliquer immédiatement le nettoyage sur les sessions existantes
-- Pour chaque utilisateur ayant plus de 10 sessions sauvegardées
DELIMITER $$

CREATE PROCEDURE apply_initial_cleanup()
BEGIN
    DECLARE done INT DEFAULT FALSE;
    DECLARE v_user_id BIGINT UNSIGNED;
    DECLARE user_cursor CURSOR FOR 
        SELECT DISTINCT user_id 
        FROM sessions 
        WHERE status = 'saved'
        GROUP BY user_id
        HAVING COUNT(*) > 10;
    DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = TRUE;

    OPEN user_cursor;
    
    read_loop: LOOP
        FETCH user_cursor INTO v_user_id;
        IF done THEN
            LEAVE read_loop;
        END IF;
        
        CALL cleanup_old_saved_sessions(v_user_id);
    END LOOP;
    
    CLOSE user_cursor;
END$$

DELIMITER ;

-- Exécuter le nettoyage initial
CALL apply_initial_cleanup();

-- Supprimer la procédure temporaire
DROP PROCEDURE IF EXISTS apply_initial_cleanup;

-- Ajouter un commentaire sur la table pour documenter la limite
ALTER TABLE sessions COMMENT = 'Sessions d\'analyse - Maximum 10 sessions sauvegardées par utilisateur';
