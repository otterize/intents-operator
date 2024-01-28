wait_for_log() {
    POD=$1
    DELAY_SECONDS=$2
    CURRENT_TIME=$(date +%s)
    END_TIME=$((CURRENT_TIME+DELAY_SECONDS))
    OUTPUT_MESSAGE="${@:3}"
    echo "Waiting for output: $OUTPUT_MESSAGE"
    echo "Waiting for $DELAY_SECONDS seconds until $END_TIME starting from $CURRENT_TIME"
    while [ $CURRENT_TIME -lt $END_TIME ];
    do
        OUTPUT=$(kubectl logs --since=10s -n otterize-tutorial-npol $POD)
        grep "$OUTPUT_MESSAGE" <(echo $OUTPUT) && return || sleep 0.2;
        CURRENT_TIME=$(date +%s)
    done
    echo "Failed to find output in logs"
    echo "Last output was: $OUTPUT"
    echo "##############################################"
    echo "##############################################"
    echo "Operator manager logs:"
    kubectl logs -n otterize-system -l app=intents-operator --tail 400
    exit 1
}

apply_intents_and_wait_for_webhook() {
    INTENTS_FILE=$1
    for i in {1..5}; do
        kubectl apply -f "$INTENTS_FILE" 2> error.txt && break;
        if grep -q "connection refused" error.txt; then
            if [ "$i" -eq 5 ]; then
                echo "Failed to apply intents, webhook server is not ready";
                cat error.txt;
                exit 1;
            fi;
            echo "Waiting for webhook to be ready, try $i/5";
            sleep 5;
        else
            cat error.txt;
            exit 1;
        fi;
    done
}
